package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/phaedrus-raznikov/nix-key/internal/pairing"
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
	RunE:  runPair,
}

func runPair(cmd *cobra.Command, args []string) error {
	iface, _ := cmd.Flags().GetString("interface")
	otel, _ := cmd.Flags().GetString("otel-endpoint")
	hostname, _ := cmd.Flags().GetString("hostname")
	expiryStr, _ := cmd.Flags().GetString("cert-expiry")
	ageKey, _ := cmd.Flags().GetString("age-key-file")
	devicesPath, _ := cmd.Flags().GetString("devices-path")
	certsDir, _ := cmd.Flags().GetString("certs-dir")
	controlSocket, _ := cmd.Flags().GetString("control-socket")
	pairInfoFile, _ := cmd.Flags().GetString("pair-info-file")

	var expiry time.Duration
	if expiryStr != "" {
		var err error
		expiry, err = time.ParseDuration(expiryStr)
		if err != nil {
			return fmt.Errorf("invalid cert-expiry %q: %w", expiryStr, err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := pairing.PairConfig{
		TailscaleInterface: iface,
		CertExpiry:         expiry,
		OTELEndpoint:       otel,
		AgeIdentityPath:    ageKey,
		HostName:           hostname,
		DevicesPath:        devicesPath,
		CertsDir:           certsDir,
		ControlSocketPath:  controlSocket,
		PairInfoFile:       pairInfoFile,
	}

	return pairing.RunPair(ctx, cfg)
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List paired devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		controlSocket, _ := cmd.Flags().GetString("control-socket")
		if controlSocket == "" {
			controlSocket = defaultControlSocket()
		}
		return runDevices(controlSocket)
	},
}

var revokeCmd = &cobra.Command{
	Use:   "revoke [device]",
	Short: "Revoke a paired device",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		controlSocket, _ := cmd.Flags().GetString("control-socket")
		if controlSocket == "" {
			controlSocket = defaultControlSocket()
		}
		return runRevoke(controlSocket, args[0], os.Stdout)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		controlSocket, _ := cmd.Flags().GetString("control-socket")
		if controlSocket == "" {
			controlSocket = defaultControlSocket()
		}
		return runStatusOrNotRunning(controlSocket)
	},
}

var exportCmd = &cobra.Command{
	Use:   "export [key-id]",
	Short: "Export an SSH public key",
	Long:  "Export an SSH public key by SHA256 fingerprint or unique prefix. Prints the key in authorized_keys format to stdout.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		controlSocket, _ := cmd.Flags().GetString("control-socket")
		if controlSocket == "" {
			controlSocket = defaultControlSocket()
		}
		return runExport(controlSocket, args[0], os.Stdout)
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, _ := cmd.Flags().GetString("config-file")
		if configPath == "" {
			configPath = defaultConfigPath()
		}
		return runConfig(configPath, os.Stdout)
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
	pairCmd.Flags().String("interface", "", "Tailscale interface name (default: tailscale0)")
	pairCmd.Flags().String("otel-endpoint", "", "OpenTelemetry collector endpoint to include in QR")
	pairCmd.Flags().String("hostname", "", "Host name to advertise (default: system hostname)")
	pairCmd.Flags().String("cert-expiry", "", "Certificate expiry duration (default: 8760h / 1 year)")
	pairCmd.Flags().String("age-key-file", "", "Path to age identity file for cert encryption")
	pairCmd.Flags().String("devices-path", "", "Path to devices.json")
	pairCmd.Flags().String("certs-dir", "", "Directory for certificate storage")
	pairCmd.Flags().String("control-socket", "", "Path to daemon control socket")
	pairCmd.Flags().String("pair-info-file", "", "Write pairing info JSON to file (for E2E testing)")

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(pairCmd)
	devicesCmd.Flags().String("control-socket", "", "Path to daemon control socket")
	rootCmd.AddCommand(devicesCmd)
	revokeCmd.Flags().String("control-socket", "", "Path to daemon control socket")
	rootCmd.AddCommand(revokeCmd)
	statusCmd.Flags().String("control-socket", "", "Path to daemon control socket")
	rootCmd.AddCommand(statusCmd)
	exportCmd.Flags().String("control-socket", "", "Path to daemon control socket")
	rootCmd.AddCommand(exportCmd)
	configCmd.Flags().String("config-file", "", "Path to config file (default: ~/.config/nix-key/config.json)")
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(testCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
