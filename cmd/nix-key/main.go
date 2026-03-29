package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "nix-key",
	Short: "Phone-as-YubiKey SSH agent",
	Long:  "nix-key uses an Android phone as a hardware-backed SSH key store, communicating over gRPC/mTLS/Tailscale.",
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the nix-key SSH agent daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("daemon: not yet implemented")
		return nil
	},
}

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair with a new phone device",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("pair: not yet implemented")
		return nil
	},
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List paired devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("devices: not yet implemented")
		return nil
	},
}

var revokeCmd = &cobra.Command{
	Use:   "revoke [device]",
	Short: "Revoke a paired device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("revoke: not yet implemented")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("status: not yet implemented")
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export [key-id]",
	Short: "Export an SSH public key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("export: not yet implemented")
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("config: not yet implemented")
		return nil
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail daemon logs in human-readable format",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("logs: not yet implemented")
		return nil
	},
}

var testCmd = &cobra.Command{
	Use:   "test [device]",
	Short: "Test connectivity to a paired device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("test: not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(pairCmd)
	rootCmd.AddCommand(devicesCmd)
	rootCmd.AddCommand(revokeCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(testCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
