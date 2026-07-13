package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"do-it-lewitt/internal/lewitt"

	"github.com/spf13/cobra"
)

func formatDB(db float64) string {
	if math.IsInf(db, -1) || db <= -120 {
		return "-∞ dB"
	}
	return fmt.Sprintf("%.1f dB", db)
}

var (
	flagUser       bool
	flagDryRun     bool
	flagDuration   int
	flagNoPlayback bool
	flagMono       bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "dilctl",
		Short: "Lewitt CONNECT 2 setup and diagnostics for Linux",
		Long:  "dilctl configures the Lewitt CONNECT 2 audio interface under Linux by installing an ALSA PCM that routes the 4-channel capture to the correct 2-channel FL/FR inputs while leaving the device available to PipeWire.",
	}

	rootCmd.AddCommand(statusCmd(), setupCmd(), wireplumberCmd(), verifyCmd(), diagnoseCmd(), teardownCmd(), resetCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func resetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset the CONNECT 2 USB device when it is stuck",
		Long:  "Deauthorizes and reauthorizes the CONNECT 2 USB device so the kernel and ALSA re-enumerate it. This command requires root privileges.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Resetting Lewitt CONNECT 2 USB device...")
			if err := lewitt.ResetUSB(); err != nil {
				return err
			}
			fmt.Println("USB device reset and re-enumerated successfully.")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current device and configuration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			dev, err := lewitt.Detect()
			if err != nil {
				return fmt.Errorf("detection failed: %w", err)
			}

			if !dev.Connected {
				fmt.Println("Lewitt CONNECT 2: NOT CONNECTED")
				fmt.Println()
				fmt.Println("Make sure the device is plugged in via USB.")
				return nil
			}

			fmt.Println("Lewitt CONNECT 2")
			fmt.Println("─────────────────────────────────────────────")
			fmt.Printf("  Card:        %s (card %d)\n", dev.CardID, dev.CardIndex)
			fmt.Printf("  USB:         %s:%s  serial %s\n", dev.VendorID, dev.ProductID, dev.Serial)
			fmt.Printf("  Product:     %s (by %s)\n", dev.Product, dev.Manufacturer)
			fmt.Println()

			capture, playback, err := lewitt.ReadStreamInfo(dev.CardID)
			if err != nil {
				fmt.Printf("  Stream info: unavailable (%v)\n", err)
			} else {
				if capture != nil {
					s := capture.Stream
					fmt.Printf("  Capture:     %dch %s %d-bit @ [%s] kHz\n",
						s.Channels, s.Format, s.Bits, strings.Join(s.Rates, ", "))
					fmt.Printf("               channel map: %s\n", s.ChannelMap)
					fmt.Printf("               state: %s\n", s.StreamState)
				}
				if playback != nil {
					s := playback.Stream
					fmt.Printf("  Playback:    %dch %s %d-bit @ [%s] kHz\n",
						s.Channels, s.Format, s.Bits, strings.Join(s.Rates, ", "))
					fmt.Printf("               channel map: %s\n", s.ChannelMap)
					fmt.Printf("               state: %s\n", s.StreamState)
				}
			}

			fmt.Println()

			status := lewitt.CheckConfig()
			fmt.Printf("  ALSA config: %s PCM", "lewitt_connect_2")
			if status.ALSAMInstalled {
				fmt.Println("  — installed ✓")
			} else {
				fmt.Println("  — NOT installed")
			}
			fmt.Printf("  PipeWire device management")
			if status.WPIgnoreInstalled {
				fmt.Println("  — ignore rule enabled ✓")
			} else if status.WPIgnoreDisabled {
				fmt.Println("  — ignore rule disabled (PipeWire enabled)")
			} else {
				fmt.Println("  — not configured")
			}

			return nil
		},
	}
}

func wireplumberCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "wireplumber", Short: "Enable or disable the WirePlumber ignore rule"}
	for _, enabled := range []bool{true, false} {
		name := "disable"
		if enabled {
			name = "enable"
		}
		value := enabled
		cmd.AddCommand(&cobra.Command{Use: name, Short: name + " the generated WirePlumber ignore rule", RunE: func(cmd *cobra.Command, args []string) error {
			target := lewitt.InstallSystem
			if flagUser {
				target = lewitt.InstallUser
			}
			return lewitt.SetWPIgnore(target, value, flagDryRun)
		}})
	}
	cmd.PersistentFlags().BoolVar(&flagUser, "user", false, "use user WirePlumber configuration")
	cmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show the change without applying it")
	return cmd
}

func setupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install ALSA config and WirePlumber ignore rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			dev, err := lewitt.Detect()
			if err != nil {
				return fmt.Errorf("detection failed: %w", err)
			}
			if !dev.Connected {
				return fmt.Errorf("Lewitt CONNECT 2 not connected. Plug it in and retry")
			}

			fmt.Printf("Setting up Lewitt CONNECT 2 (card %s)...\n", dev.CardID)
			fmt.Println()

			target := lewitt.InstallSystem
			if flagUser {
				target = lewitt.InstallUser
			}
			fmt.Printf("Install target: %s\n", target)
			fmt.Println()

			return lewitt.InstallConfig(target, flagDryRun)
		},
	}
	cmd.Flags().BoolVar(&flagUser, "user", false, "install to user config (~/.asoundrc.d, ~/.config/wireplumber) instead of system-wide")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be done without writing anything")
	return cmd
}

func verifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Record and playback test to confirm the device works",
		RunE: func(cmd *cobra.Command, args []string) error {
			dev, err := lewitt.Detect()
			if err != nil {
				return fmt.Errorf("detection failed: %w", err)
			}
			if !dev.Connected {
				return fmt.Errorf("Lewitt CONNECT 2 not connected")
			}

			if err := lewitt.ValidateConfig(); err != nil {
				return fmt.Errorf("%w\nRun 'dilctl setup' first", err)
			}

			fmt.Printf("Recording %d second(s) from lewitt_connect_2...\n", flagDuration)
			fmt.Println("(Make some sound into the microphone!)")
			fmt.Println()

			result, err := lewitt.Verify(flagDuration, flagNoPlayback, flagMono)
			if err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("Verification results:")
			fmt.Println("─────────────────────────────────────────────")

			if result.CaptureOK {
				fmt.Println("  Capture:     PASS ✓")
				fmt.Printf("  Channel FL:  %s\n", formatDB(result.CaptureRMSDB[0]))
				fmt.Printf("  Channel FR:  %s\n", formatDB(result.CaptureRMSDB[1]))

				if result.CaptureRMSDB[0] < -60 && result.CaptureRMSDB[1] < -60 {
					fmt.Println("  ⚠  Signal below -60 dB on both channels.")
					fmt.Println("     Check that a mic is connected and gain is turned up.")
				} else if result.CaptureRMSDB[0] < -60 {
					fmt.Println("  ⚠  Channel FL very quiet — check left/first XLR input.")
				} else if result.CaptureRMSDB[1] < -60 {
					fmt.Println("  ⚠  Channel FR very quiet — check right/second XLR input.")
				}
			} else {
				fmt.Println("  Capture:     FAIL ✗")
				if result.CaptureError != "" {
					fmt.Printf("  Error:       %s\n", result.CaptureError)
				}
			}

			if !flagNoPlayback {
				fmt.Println()
				if result.PlaybackOK {
					fmt.Println("  Playback:    PASS ✓")
					fmt.Println("  (You should have heard the recording through headphones.)")
				} else {
					fmt.Println("  Playback:    FAIL ✗")
					if result.PlaybackError != "" {
						fmt.Printf("  Error:       %s\n", result.PlaybackError)
					}
				}
			}

			fmt.Println()
			fmt.Printf("  WAV file:    %s\n", result.WavFile)

			return nil
		},
	}
	cmd.Flags().IntVarP(&flagDuration, "duration", "d", 2, "recording duration in seconds")
	cmd.Flags().BoolVar(&flagNoPlayback, "no-playback", false, "skip playback test")
	cmd.Flags().BoolVar(&flagMono, "mono", false, "mix playback to mono on both channels")
	return cmd
}

func diagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose",
		Short: "Dump full diagnostic information",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := lewitt.Diagnose()
			if err != nil {
				return err
			}
			fmt.Print(output)
			return nil
		},
	}
}

func teardownCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove dilctl config and restore WirePlumber management",
		RunE: func(cmd *cobra.Command, args []string) error {
			return lewitt.TeardownConfig(flagDryRun)
		},
	}
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be removed without removing anything")
	return cmd
}
