package container

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"
	"runtime"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	log "github.com/sirupsen/logrus"
	api "github.com/weaveworks/ignite/pkg/apis/ignite"
	"github.com/weaveworks/ignite/pkg/constants"
	"github.com/weaveworks/ignite/pkg/logs"
	"github.com/weaveworks/ignite/pkg/util"
)

// ExecuteFirecracker executes the firecracker process using the Go SDK
func ExecuteFirecracker(vm *api.VM, dhcpIfaces []DHCPInterface) error {
	log.Warnf("entering firecrcker")
	drivePath := vm.SnapshotDev()

	networkInterfaces := make([]firecracker.NetworkInterface, 0, len(dhcpIfaces))
	for _, dhcpIface := range dhcpIfaces {
		networkInterfaces = append(networkInterfaces, firecracker.NetworkInterface{
			MacAddress:  dhcpIface.MACFilter,
			HostDevName: dhcpIface.VMTAP,
		})
	}

	vCPUCount := int64(vm.Spec.CPUs)
	memSizeMib := int64(vm.Spec.Memory.MBytes())

	cmdLine := vm.Spec.Kernel.CmdLine
	if len(cmdLine) == 0 {
		// if for some reason cmdline would be unpopulated, set it to the default
		cmdLine = constants.VM_DEFAULT_KERNEL_ARGS
	}

	// Convert the logrus error level to a Firecracker compatible error level.
	// Firecracker accepts "Error", "Warning", "Info", and "Debug", case-sensitive.
	fcLogLevel := "Debug"
	switch logs.Logger.Level {
	case log.InfoLevel:
		fcLogLevel = "Info"
	case log.WarnLevel:
		fcLogLevel = "Warning"
	case log.ErrorLevel, log.FatalLevel, log.PanicLevel:
		fcLogLevel = "Error"
	}

	firecrackerSocketPath := path.Join(vm.ObjectPath(), constants.FIRECRACKER_API_SOCKET)
	logSocketPath := path.Join(vm.ObjectPath(), constants.LOG_FIFO)
	metricsSocketPath := path.Join(vm.ObjectPath(), constants.METRICS_FIFO)
	cfg := firecracker.Config{
		SocketPath:      firecrackerSocketPath,
		KernelImagePath: constants.IGNITE_SPAWN_VMLINUX_FILE_PATH,
		KernelArgs:      cmdLine,
		Drives: []models.Drive{{
			DriveID:      firecracker.String("1"),
			IsReadOnly:   firecracker.Bool(false),
			IsRootDevice: firecracker.Bool(true),
			PathOnHost:   &drivePath,
		}},
		NetworkInterfaces: networkInterfaces,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  &vCPUCount,
			MemSizeMib: &memSizeMib,
			HtEnabled:  firecracker.Bool(true),
		},
		//JailerCfg: firecracker.JailerConfig{
		//	GID:      firecracker.Int(0),
		//	UID:      firecracker.Int(0),
		//	ID:       vm.ID,
		//	NumaNode: firecracker.Int(0),
		//	ExecFile: "firecracker",
		//},

		LogLevel: fcLogLevel,
		// TODO: We could use /dev/null, but firecracker-go-sdk issues Mkfifo which collides with the existing device
		LogFifo:     logSocketPath,
		MetricsFifo: metricsSocketPath,
	}

	// Add the volumes to the VM
	for i, volume := range vm.Spec.Storage.Volumes {
		volumePath := path.Join(constants.IGNITE_SPAWN_VOLUME_DIR, volume.Name)
		if !util.FileExists(volumePath) {
			log.Warnf("Skipping nonexistent volume: %q", volume.Name)
			continue // Skip all nonexistent volumes
		}

		cfg.Drives = append(cfg.Drives, models.Drive{
			DriveID:      firecracker.String(strconv.Itoa(i + 2)),
			IsReadOnly:   firecracker.Bool(false), // TODO: Support read-only volumes
			IsRootDevice: firecracker.Bool(false),
			PathOnHost:   &volumePath,
		})
	}

	// Remove these FIFOs for now
	defer os.Remove(logSocketPath)
	defer os.Remove(metricsSocketPath)

	ctx, vmmCancel := context.WithCancel(context.Background())
	defer vmmCancel()

	cmd := firecracker.VMCommandBuilder{}.
		WithBin("firecracker").
		WithSocketPath(firecrackerSocketPath).
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	m, err := firecracker.NewMachine(ctx, cfg, firecracker.WithProcessRunner(cmd))
	if err != nil {
		return fmt.Errorf("failed to create machine: %s", err)
	}

	//defer os.Remove(cfg.SocketPath)

	//if opts.validMetadata != nil {
	//	m.EnableMetadata(opts.validMetadata)
	//}

	if err := m.Start(ctx); err != nil {
		return fmt.Errorf("failed to start machine: %v", err)
	}
	defer m.StopVMM()

	installSignalHandlers(ctx, m)

        // Create file names for persistent files to save FIFO stream to
        logSocketPathPersistent := path.Join(vm.ObjectPath(), constants.LOG_FIFO_PERSISTENT)
        metricsSocketPathPersistent := path.Join(vm.ObjectPath(), constants.METRICS_FIFO_PERSISTENT)

        runtime.GOMAXPROCS(3)

	// start goroutines to watch NamedPipe and cp any writes to persistent logs
        go util.NamedPipeWatcher(logSocketPath, logSocketPathPersistent)
        go util.NamedPipeWatcher(metricsSocketPath, metricsSocketPathPersistent)

	// wait for the VMM to exit
	if err := m.Wait(ctx); err != nil {
		return fmt.Errorf("wait returned an error %s", err)
	}

	return nil
}

// Install custom signal handlers:
func installSignalHandlers(ctx context.Context, m *firecracker.Machine) {
	go func() {
		// Clear some default handlers installed by the firecracker SDK:
		signal.Reset(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				fmt.Println("Caught SIGTERM, requesting clean shutdown")
				m.Shutdown(ctx)
				time.Sleep(constants.STOP_TIMEOUT * time.Second)

				// There's no direct way of checking if a VM is running, so we test if we can send it another shutdown
				// request. If that fails, the VM is still running and we need to kill it.
				if err := m.Shutdown(ctx); err == nil {
					fmt.Println("Timeout exceeded, forcing shutdown") // TODO: Proper logging
					m.StopVMM()
				}
			case s == syscall.SIGQUIT:
				fmt.Println("Caught SIGQUIT, forcing shutdown")
				m.StopVMM()
			}
		}
	}()
}
