package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch [input]",
	Short: "Switch monitor input",
	Long:  "Switch the monitor to a specified input (hdmi, usb-c, etc.)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Your implementation here
		input := args[0]
		// TODO: Actual switching logic will come later
		if verbose {
			fmt.Println("Verbose mode enabled: Switching monitor input...")
		} else {
			fmt.Printf("Switching to input: %s\n", input)

		}
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
