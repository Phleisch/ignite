package cmd

import (
	"io"
	"os"
	"os/signal"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/gitops-toolkit/pkg/filter"
	"github.com/weaveworks/ignite/pkg/providers"
	"github.com/weaveworks/ignite/pkg/operations"
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

			ticker := time.NewTicker(100 * time.Millisecond)

			// goroutine to check status of VM every 100 ms
			// cleanup VM if VM is no longer running
			go func() {
				log.Infof("Watching %s...", vmName)
				for {
					select {
					case <- ticker.C:
						start := time.Now()
						vm, err = providers.Client.VMs().Find(filter.NewIDNameFilter(vmName))
						log.Infof("Took %s", time.Since(start))
						if vm.Running() {
							log.Infof("VM is running")
							break
						}

						log.Infof("VM is stopped, removing...")
						if err = operations.DeleteVM(providers.Client, vm); err != nil {
						//if err = operations.CleanupVM(vm); err != nil {
								log.Fatalf("Error deleting VM:, %s", err)
						}
						ticker.Stop()
						endWaiter.Done()
						return
					case <- signalChannel:
						ticker.Stop()
						endWaiter.Done()
						return
					}
				}
			}()

			//go func() {
			//	<-quit
			//	<-signalChannel
			//	endWaiter.Done()
			//}()

			endWaiter.Wait()
		},
	}

	return cmd
}
