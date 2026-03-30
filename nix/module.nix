{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.nix-key;

  # When jaeger.enable is true, otelEndpoint is automatically set to 127.0.0.1:4317.
  # Uses 127.0.0.1 instead of localhost to avoid IPv6 resolution issues in NixOS VMs
  # where localhost resolves to ::1 first but Jaeger only listens on IPv4 (0.0.0.0).
  # The assertion ensures manual otelEndpoint is null when jaeger is enabled.
  effectiveOtelEndpoint =
    if cfg.tracing.jaeger.enable then "127.0.0.1:4317" else cfg.tracing.otelEndpoint;

  # Jaeger v2 requires an explicit config file to set up the in-memory storage
  # pipeline correctly. Without it, the default exporter pipeline fails with
  # "traces export: context deadline exceeded".
  jaegerConfig = pkgs.writeText "jaeger-config.yaml" ''
    extensions:
      jaeger_storage:
        backends:
          memstore:
            memory:
              max_traces: 10000
      jaeger_query:
        storage:
          traces: memstore
        ui:
          config_file: ""

    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: 0.0.0.0:4317
          http:
            endpoint: 0.0.0.0:4318

    exporters:
      jaeger_storage_exporter:
        trace_storage: memstore
      debug:
        verbosity: detailed

    service:
      extensions: [jaeger_storage, jaeger_query]
      pipelines:
        traces:
          receivers: [otlp]
          exporters: [jaeger_storage_exporter, debug]
  '';

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

  # Store-based config.json written from module options.
  # socketPath and controlSocketPath are omitted when empty (default) — they
  # are resolved at runtime from $XDG_RUNTIME_DIR in the preStart script
  # and passed via environment variables.
  configFile = pkgs.writeText "nix-key-config.json" (
    builtins.toJSON (
      {
        port = cfg.port;
        tailscaleInterface = cfg.tailscaleInterface;
        allowKeyListing = cfg.allowKeyListing;
        signTimeout = cfg.signTimeout;
        connectionTimeout = cfg.connectionTimeout;
        logLevel = cfg.logLevel;
        otelEndpoint = effectiveOtelEndpoint;
        jaegerEnable = cfg.tracing.jaeger.enable;
        ageKeyFile = cfg.secrets.ageKeyFile;
        tailscaleAuthKeyFile = cfg.tailscale.authKeyFile;
        certExpiry = cfg.certExpiry;
        devices = lib.mapAttrs (_name: dev: {
          inherit (dev)
            name
            tailscaleIp
            port
            certFingerprint
            ;
          clientCertPath = if dev.clientCert != null then toString dev.clientCert else null;
          clientKeyPath = if dev.clientKey != null then toString dev.clientKey else null;
        }) cfg.devices;
      }
      // lib.optionalAttrs (cfg.socketPath != "") {
        socketPath = cfg.socketPath;
      }
      // lib.optionalAttrs (cfg.controlSocketPath != "") {
        controlSocketPath = cfg.controlSocketPath;
      }
    )
  );
in
{
  options.services.nix-key = {
    enable = lib.mkEnableOption "nix-key SSH agent that delegates signing to a paired Android phone over Tailscale with mTLS";

    package = lib.mkOption {
      type = lib.types.package;
      description = "The nix-key package to use.";
    };

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
      default = "";
      description = ''
        Path to the Unix socket for the SSH agent protocol.
        When empty (default), the daemon uses $XDG_RUNTIME_DIR/nix-key/agent.sock
        which is resolved at runtime by the systemd preStart script.
      '';
    };

    controlSocketPath = lib.mkOption {
      type = lib.types.str;
      default = "";
      description = ''
        Path to the Unix socket for the daemon control protocol (status queries, revoke, etc.).
        When empty (default), the daemon uses $XDG_RUNTIME_DIR/nix-key/control.sock
        which is resolved at runtime by the systemd preStart script.
      '';
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

        package = lib.mkOption {
          type = lib.types.package;
          description = "The Jaeger package to use.";
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

    # systemd user service: nix-key-agent
    systemd.user.services.nix-key-agent = {
      description = "nix-key SSH agent — delegates signing to paired Android phone over Tailscale";
      after = [ "network.target" ];
      wantedBy = [ "default.target" ];

      serviceConfig = {
        ExecStart = "${lib.getExe cfg.package} daemon --config %h/.config/nix-key/config.json";
        Restart = "on-failure";
        RestartSec = 5;

        # Create ~/.config/nix-key/ and ~/.local/state/nix-key/ with 0700
        ConfigurationDirectory = "nix-key";
        ConfigurationDirectoryMode = "0700";
        StateDirectory = "nix-key";
        StateDirectoryMode = "0700";
        RuntimeDirectory = "nix-key";
        RuntimeDirectoryMode = "0700";

        # Pick up resolved socket paths written by preStart
        EnvironmentFile = "-%t/nix-key/env";
      };

      # Create certs subdirectory, symlink config.json, and resolve socket
      # paths before starting the daemon. Socket paths are written to an
      # EnvironmentFile so they can use $RUNTIME_DIRECTORY (set by systemd
      # from RuntimeDirectory=nix-key). The - prefix on EnvironmentFile makes
      # it optional (no error if missing on first boot).
      preStart =
        let
          socketLine =
            if cfg.socketPath != "" then
              "NIXKEY_SOCKET_PATH=${cfg.socketPath}"
            else
              "NIXKEY_SOCKET_PATH=$RUNTIME_DIRECTORY/agent.sock";
          controlSocketLine =
            if cfg.controlSocketPath != "" then
              "NIXKEY_CONTROL_SOCKET_PATH=${cfg.controlSocketPath}"
            else
              "NIXKEY_CONTROL_SOCKET_PATH=$RUNTIME_DIRECTORY/control.sock";
        in
        ''
          mkdir -p -m 0700 "$STATE_DIRECTORY/certs"
          ln -sf ${configFile} "$CONFIGURATION_DIRECTORY/config.json"
          printf '%s\n' "${socketLine}" "${controlSocketLine}" > "$RUNTIME_DIRECTORY/env"
        '';

      environment = {
        NIXKEY_LOG_LEVEL = cfg.logLevel;
      }
      // lib.optionalAttrs (cfg.socketPath != "") {
        NIXKEY_SOCKET_PATH = cfg.socketPath;
      }
      // lib.optionalAttrs (cfg.controlSocketPath != "") {
        NIXKEY_CONTROL_SOCKET_PATH = cfg.controlSocketPath;
      }
      // lib.optionalAttrs (effectiveOtelEndpoint != null) {
        NIXKEY_OTEL_ENDPOINT = effectiveOtelEndpoint;
      };
    };

    # Create /etc/xdg/environment.d/50-nix-key.conf so that
    # SSH_AUTH_SOCK is available in all user login sessions.
    # environment.d supports variable expansion, so ${XDG_RUNTIME_DIR} works.
    environment.etc."xdg/environment.d/50-nix-key.conf" = {
      text =
        let
          sock = if cfg.socketPath != "" then cfg.socketPath else "\${XDG_RUNTIME_DIR}/nix-key/agent.sock";
        in
        ''
          SSH_AUTH_SOCK=${sock}
        '';
      mode = "0644";
    };

    # Jaeger all-in-one service for local trace collection (FR-068).
    # Runs as a system service listening on OTLP gRPC (4317) and HTTP query (16686).
    systemd.services.jaeger-all-in-one = lib.mkIf cfg.tracing.jaeger.enable {
      description = "Jaeger all-in-one tracing backend for nix-key";
      after = [ "network.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        ExecStart = "${cfg.tracing.jaeger.package}/bin/jaeger --config ${jaegerConfig}";
        Restart = "on-failure";
        RestartSec = 5;
        DynamicUser = true;
        # Jaeger stores traces in memory by default (sufficient for dev/debug).
        # Limit memory usage to avoid runaway growth.
        MemoryMax = "512M";
      };
    };
  };
}
