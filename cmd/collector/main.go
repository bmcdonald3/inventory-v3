package main

import (
	"fmt"
	"os"

	"github.com/example/inventory-v3/pkg/collector"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "collector",
	Short: "Gathers hardware inventory via Redfish and posts it to the OpenCHAMI API.",
	Run:   executeGatherAndPost,
}

var bmcIP string

func init() {
	// Define the --ip flag for the BMC IP
	rootCmd.Flags().StringVarP(&bmcIP, "ip", "i", "", "The IP address of the BMC to gather inventory from (required)")
	rootCmd.MarkFlagRequired("ip")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// executeGatherAndPost is the main function logic triggered by cobra.
func executeGatherAndPost(cmd *cobra.Command, args []string) {
	fmt.Printf("Starting inventory collection for BMC IP: %s\n", bmcIP)

	err := collector.CollectAndPost(bmcIP)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Collection Failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Inventory collection and posting completed successfully.")
}