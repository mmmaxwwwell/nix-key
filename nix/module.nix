{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.nix-key;

  deviceSubmodule = lib.types.submodule {
    options = {
      name = lib.mkOption {
        type = lib.types.str;
        description = "Display name of the paired phone (e.g. \"Pixel 8\").";
      };

      tailscaleIp = lib.mkOption {
        type = lib.types.str;
        description = "Tailscale IP address of the phone.";
      };

      port = lib.mkOption {
        type = lib.types.port;
        default = 29418;
        description = "Port the phone's gRPC server listens on.";
      };

      certFingerprint = lib.mkOption {
        type = lib.types.str;
        description = "SHA256 fingerprint of the phone's TLS server certificate. Primary identity for cert pinning.";
      };

      clientCert = lib.mkOption {
        type = lib.types.nullOr lib.types.path;
        default = null;
        description = ''
          Path to the host's client certificate (PEM) for mTLS with this device.
          If null, the certificate is expected to be set during the pairing flow (FR-065).
        '';
      };

      clientKey = lib.mkOption {
        type = lib.types.nullOr lib.types.path;
        default = null;
        description = ''
          Path to the host's client private key (PEM, age-encrypted) for mTLS with this device.
          If null, the key is expected to be set during the pairing flow (FR-065).
        '';
      };
    };
  };

  # Build the config.json structure from module options
  configJson = builtins.toJSON {
    port = cfg.port;
    tailscaleInterface = cfg.tailscaleInterface;
    allowKeyListing = cfg.allowKeyListing;
    signTimeout = cfg.signTimeout;
    connectionTimeout = cfg.connectionTimeout;
    socketPath = cfg.socketPath;
    logLevel = cfg.logLevel;
    otelEndpoint = cfg.tracing.otelEndpoint;
    jaegerEnable = cfg.tracing.jaeger.enable;
    ageKeyFile = cfg.secrets.ageKeyFile;
    tailscaleAuthKeyFile = cfg.tailscale.authKeyFile;
    certExpiry = cfg.certExpiry;
    devices = lib.mapAttrs (
      _name: dev: {
        inherit (dev) name tailscaleIp port certFingerprint;
        clientCertPath =
          if dev.clientCert != null then toString dev.clientCert else null;
        clientKeyPath =
          if dev.clientKey != null then toString dev.clientKey else null;
      }
    ) cfg.devices;
  };
in
{
  options.services.nix-key = {
    enable = lib.mkEnableOption "nix-key SSH agent that delegates signing to a paired Android phone over Tailscale with mTLS";

    port = lib.mkOption {
      type = lib.types.port;
      default = 29418;
      description = "Default port for phone gRPC connections. Can be overridden per device.";
    };

    tailscaleInterface = lib.mkOption {
      type = lib.types.str;
      default = "tailscale0";
      description = "Network interface name for Tailscale. The daemon only initiates mTLS connections via this interface.";
    };

    allowKeyListing = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = ''
        Whether the host allows listing SSH keys from paired phones.
        When false, the daemon returns an empty key list without contacting phones.

        Note: the phone can independently deny key listing in its own settings,
        which results in an empty list from that phone even when the host allows
        listing (FR-066).
      '';
    };

    signTimeout = lib.mkOption {
      type = lib.types.ints.positive;
      default = 30;
      description = "Seconds to wait for the phone user to approve or deny a sign request.";
    };

    connectionTimeout = lib.mkOption {
      type = lib.types.ints.positive;
      default = 10;
      description = "Seconds to wait for an mTLS connection to a phone before giving up.";
    };

    socketPath = lib.mkOption {
      type = lib.types.str;
      default = "\${XDG_RUNTIME_DIR}/nix-key/agent.sock";
      description = "Path to the Unix socket for the SSH agent protocol.";
    };

    logLevel = lib.mkOption {
      type = lib.types.enum [
        "debug"
        "info"
        "warn"
        "error"
        "fatal"
      ];
      default = "info";
      description = "Minimum log level for the daemon. One of: debug, info, warn, error, fatal.";
    };

    tracing = {
      otelEndpoint = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        description = ''
          OTLP collector endpoint for OpenTelemetry trace export (e.g. "localhost:4317").
          When null, tracing is disabled with zero overhead.
        '';
      };

      jaeger = {
        enable = lib.mkOption {
          type = lib.types.bool;
          default = false;
          description = "Whether to run a local Jaeger instance and configure the daemon to export traces to it.";
        };
      };
    };

    secrets = {
      ageKeyFile = lib.mkOption {
        type = lib.types.str;
        default = "~/.local/state/nix-key/age-identity.txt";
        description = "Path to the age identity file used for decrypting mTLS private keys at rest (FR-103).";
      };
    };

    tailscale = {
      authKeyFile = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        description = "Path to a file containing a pre-authorized Tailscale auth key. Useful for automated setups and testing (FR-013b).";
      };
    };

    certExpiry = lib.mkOption {
      type = lib.types.str;
      default = "365d";
      description = ''
        Expiry duration for generated mTLS certificates (e.g. "365d", "90d").
        Re-pairing is required to rotate expired certificates.
      '';
    };

    devices = lib.mkOption {
      type = lib.types.attrsOf deviceSubmodule;
      default = { };
      description = ''
        Declarative device definitions. These are merged with runtime-paired devices
        from ~/.local/state/nix-key/devices.json at daemon startup. Nix-declared
        devices take precedence for cert paths; runtime values win for lastSeen
        and tailscaleIp.
      '';
      example = lib.literalExpression ''
        {
          pixel-8 = {
            name = "Pixel 8";
            tailscaleIp = "100.64.0.2";
            port = 29418;
            certFingerprint = "sha256:abc123...";
          };
        }
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.tracing.jaeger.enable -> cfg.tracing.otelEndpoint == null;
        message = "services.nix-key.tracing.otelEndpoint must not be set when jaeger.enable is true; the module sets it automatically.";
      }
    ];
  };
}
