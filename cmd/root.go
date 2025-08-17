package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "monitorswitch [command]",
	Short: "A cross-platform monitor control tool",
	Long: `MonitorSwitch allows you to control monitor settings like input switching,
brightness, and contrast across Linux, macOS, and Windows using DDC/CI protocol.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	// Your code here - what should happen if command execution fails?
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

}

func init() {
	// This is where you'll add global flags later
}
