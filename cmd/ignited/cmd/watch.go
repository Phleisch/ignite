package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"path"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/gitops-toolkit/pkg/filter"
	"github.com/weaveworks/ignite/pkg/providers"
	"github.com/weaveworks/ignite/pkg/operations"
	"github.com/rjeczalik/notify"
	"github.com/weaveworks/ignite/pkg/constants"
)

func NewCmdWatch(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch <vm>",
		Short: "Watches a specified VM and performs cleanup once the VM has stopped.",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			vmName := args[0]

			// Check if VM exists
			vm, err := providers.Client.VMs().Find(filter.NewIDNameFilter(vmName))
			if err != nil {
				log.Fatalf("Could not find VM %s.", vmName)
				return
			}


			// Wait for Ctrl + C
			var endWaiter sync.WaitGroup
			endWaiter.Add(1)

			signalChannel := make(chan os.Signal, 1)
			signal.Notify(signalChannel, os.Interrupt)

			path := path.Join(vm.ObjectPath(), constants.LOG_FIFO)

			c := make(chan notify.EventInfo, 1)
			notify.Watch(path, c, notify.Remove)

			// goroutine to check status of VM every 100 ms
			// cleanup VM if VM is no longer running
			go func() {
			    log.Infof("Watching %s...", vmName)
				for {
					select {
					case e := <-c:
						if e.Event() != notify.Remove {
							break
						}
						vm, err = providers.Client.VMs().Find(filter.NewIDNameFilter(vmName))
						if vm.Running() {
							log.Infof("Unexpected VM is still running")
							break
						}

						//if err = operations.DeleteVM(providers.Client, vm); err != nil {
						c := providers.Client
						if err := c.VMs().Delete(vm.GetUID()); err != nil {
							//log.Infof("Caught err %v", err)
						}

						if err = operations.CleanupVM(vm); err != nil {
							log.Fatalf("%s", fmt.Errorf("Error deleting VM: %v", err))
						}
						endWaiter.Done()
						return
					case <- signalChannel:
						log.Infof("ctrl-c")
						endWaiter.Done()
						return
					}
				}
			}()

			endWaiter.Wait()
		},
	}

	return cmd
}
