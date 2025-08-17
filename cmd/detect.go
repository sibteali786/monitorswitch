package cmd

import "github.com/spf13/cobra"

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detects monitors connected",
	Long:  "Gets the list of monitors connected to the system and their current input sources.",
	Run: func(cmd *cobra.Command, args []string) {
		// Your implementation here
		// For now, we will just print a placeholder message
		if verbose {
			println("verbose mode enabled: Detecting connected monitors")
			println("Monitor 1: HDMI [x]")
			println("Monitor 2: USB-C [ ]")
		} else {

			println("Detecting connected monitors...")
			println("Monitor 1: HDMI")
		}
	},
}

func init() {
	rootCmd.AddCommand(detectCmd)
}
