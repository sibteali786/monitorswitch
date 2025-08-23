package cmd

import (
	"fmt"
	"monitorswitch/internal/ddc"

	"github.com/spf13/cobra"
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detects monitors connected",
	Long:  "Gets the list of monitors connected to the system and their current input sources.",
	Run: func(cmd *cobra.Command, args []string) {
		// Your implementation here
		detector := ddc.NewDetector()

		fmt.Printf("Operating System: %s\n", detector.GetOSInfo())

		supported, message := detector.CheckDDCSupport()
		if supported {
			fmt.Printf("✓ DDC/CI Support: %s\n", message)
		} else {
			fmt.Printf("✗ DDC/CI Support: %s\n", message)
		}
		// For now, we will just print a placeholder message
		if verbose {
			fmt.Println("\n[VERBOSE] Attempting monitor detection...")
		}

		monitors, err := detector.DetectMonitors()
		if err != nil {
			fmt.Printf("x Monitor Detection Failed: %v\n", err)
		}

		if len(monitors) == 0 {
			fmt.Println("\nNo DDC/CI compatible monitors detected")
			if verbose {
				fmt.Println("[VERBOSE] This could mean:")
				fmt.Println("  - No external monitors connected")
				fmt.Println("  - Monitors don't support DDC/CI")
				fmt.Println("  - DDC/CI tools not properly configured")
			}
		}

		fmt.Printf("\nFound %d monitors", len(monitors))
		for i, monitor := range monitors {
			fmt.Printf("- Monitor %d: %s (ID: %s)\n", i+1, monitor.Name, monitor.ID)
			if monitor.CurrentInput != "" {
				fmt.Printf("  Current input: %s\n", monitor.CurrentInput)
			}

			if verbose && len(monitor.Inputs) > 0 {
				fmt.Printf("  Available inputs: ")
				for input, code := range monitor.Inputs {
					fmt.Printf("%s (0x%02X) ", input, code)
				}
				fmt.Println()
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(detectCmd)
}
