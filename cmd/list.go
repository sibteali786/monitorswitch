package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists available inputs",
	Long:  "Lists all available inputs like (hdmi, usb-c, etc.)",
	Run: func(cmd *cobra.Command, args []string) {
		// Your implementation here
		if verbose {
			fmt.Println(" (Verbose mode enabled: Listing all available inputs in detail...)")
		} else {
			fmt.Println("Available inputs: HDMI, USB-C, DisplayPort")
		}
	},
}

func init() {

	rootCmd.AddCommand(listCmd)
}
