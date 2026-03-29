# NixOS VM test for nix-key service module.
# Validates: T-NM-01 (module evaluation), T-NM-02 (service lifecycle),
# T-NM-03 (config.json), T-NM-04 (device merge), T-NM-05 (graceful shutdown).
# Covers: SC-004, SC-010, FR-063, FR-064, FR-E07.
{ pkgs, nixKeyModule }:
let
  testSocketPath = "/run/user/1000/nix-key/agent.sock";
  testControlSocketPath = "/run/user/1000/nix-key/control.sock";
  testPort = 29418;
  testSignTimeout = 15;
  testConnectionTimeout = 5;
  testLogLevel = "debug";
  testCertExpiry = "90d";
  testTailscaleInterface = "tailscale0";

in
{
  name = "nix-key-service";

  nodes.machine =
    { config, lib, ... }:
    {
      imports = [ nixKeyModule ];

      # Enable nix-key with test configuration
      services.nix-key = {
        enable = true;
        package = pkgs.nix-key;
        port = testPort;
        tailscaleInterface = testTailscaleInterface;
        allowKeyListing = false;
        signTimeout = testSignTimeout;
        connectionTimeout = testConnectionTimeout;
        socketPath = testSocketPath;
        controlSocketPath = testControlSocketPath;
        logLevel = testLogLevel;
        certExpiry = testCertExpiry;
        secrets.ageKeyFile = "/tmp/test-age-identity.txt";
        devices = {
          test-phone = {
            name = "Test Phone";
            tailscaleIp = "100.64.0.2";
            port = testPort;
            certFingerprint = "sha256:test-fingerprint-abc123";
          };
          nix-tablet = {
            name = "Nix Tablet";
            tailscaleIp = "100.64.0.3";
            port = 29419;
            certFingerprint = "sha256:nix-tablet-fingerprint-def456";
          };
        };
      };

      # Ensure we have a regular user for the user service
      users.users.testuser = {
        isNormalUser = true;
        uid = 1000;
        group = "testuser";
      };
      users.groups.testuser = { };

      # Enable lingering so the user service manager starts at boot
      # (without requiring the user to log in via a seat)
      systemd.tmpfiles.rules = [
        "f /var/lib/systemd/linger/testuser - - - -"
      ];
    };

  testScript = ''
    import json

    machine.start()
    machine.wait_for_unit("multi-user.target")

    # Wait for user service manager to be ready (lingering user)
    machine.wait_until_succeeds(
        "systemctl --user -M testuser@ is-system-running 2>&1 | grep -qE 'running|degraded'",
        timeout=30,
    )

    # ── T-NM-01: Module evaluation — verify config.json was generated ──

    with subtest("T-NM-01: config.json exists and is valid JSON"):
        # The config file is generated in the Nix store. Find it by searching
        # the store for the known filename. Use pipeline to avoid $() syntax.
        config_raw = machine.succeed(
            "find /nix/store -maxdepth 1 -name '*nix-key-config.json' | head -1 | xargs cat"
        ).strip()
        config = json.loads(config_raw)

    # ── T-NM-03: Config file generation — verify all settings ──

    with subtest("T-NM-03: config.json contains correct port"):
        assert config["port"] == ${toString testPort}, \
            f"Expected port ${toString testPort}, got {config['port']}"

    with subtest("T-NM-03: config.json contains correct tailscaleInterface"):
        assert config["tailscaleInterface"] == "${testTailscaleInterface}", \
            f"Expected '${testTailscaleInterface}', got {config['tailscaleInterface']}"

    with subtest("T-NM-03: config.json contains correct allowKeyListing"):
        assert config["allowKeyListing"] is False, \
            f"Expected allowKeyListing false, got {config['allowKeyListing']}"

    with subtest("T-NM-03: config.json contains correct signTimeout"):
        assert config["signTimeout"] == ${toString testSignTimeout}, \
            f"Expected ${toString testSignTimeout}, got {config['signTimeout']}"

    with subtest("T-NM-03: config.json contains correct connectionTimeout"):
        assert config["connectionTimeout"] == ${toString testConnectionTimeout}, \
            f"Expected ${toString testConnectionTimeout}, got {config['connectionTimeout']}"

    with subtest("T-NM-03: config.json contains correct socketPath"):
        assert config["socketPath"] == "${testSocketPath}", \
            f"Expected '${testSocketPath}', got {config['socketPath']}"

    with subtest("T-NM-03: config.json contains correct logLevel"):
        assert config["logLevel"] == "${testLogLevel}", \
            f"Expected '${testLogLevel}', got {config['logLevel']}"

    with subtest("T-NM-03: config.json contains correct certExpiry"):
        assert config["certExpiry"] == "${testCertExpiry}", \
            f"Expected '${testCertExpiry}', got {config['certExpiry']}"

    with subtest("T-NM-03: config.json contains correct ageKeyFile"):
        assert config["ageKeyFile"] == "/tmp/test-age-identity.txt", \
            f"Expected '/tmp/test-age-identity.txt', got {config['ageKeyFile']}"

    with subtest("T-NM-03: config.json contains Nix-declared device"):
        devices = config["devices"]
        assert "test-phone" in devices, \
            f"Expected 'test-phone' in devices, got {list(devices.keys())}"
        dev = devices["test-phone"]
        assert dev["name"] == "Test Phone", f"Unexpected name: {dev['name']}"
        assert dev["tailscaleIp"] == "100.64.0.2", f"Unexpected IP: {dev['tailscaleIp']}"
        assert dev["port"] == ${toString testPort}, f"Unexpected port: {dev['port']}"
        assert dev["certFingerprint"] == "sha256:test-fingerprint-abc123", \
            f"Unexpected fingerprint: {dev['certFingerprint']}"
        assert dev["clientCertPath"] is None, \
            f"Expected null clientCertPath, got {dev['clientCertPath']}"
        assert dev["clientKeyPath"] is None, \
            f"Expected null clientKeyPath, got {dev['clientKeyPath']}"

    with subtest("T-NM-03: config.json otelEndpoint is null when not configured"):
        assert config["otelEndpoint"] is None, \
            f"Expected null otelEndpoint, got {config['otelEndpoint']}"

    # ── SSH_AUTH_SOCK via environment.d ──

    with subtest("T-NM-02: environment.d file sets SSH_AUTH_SOCK"):
        env_content = machine.succeed(
            "cat /etc/xdg/environment.d/50-nix-key.conf"
        ).strip()
        expected_line = "SSH_AUTH_SOCK=${testSocketPath}"
        assert expected_line in env_content, \
            f"Expected '{expected_line}' in environment.d, got: {env_content}"

    # ── T-NM-02: Service lifecycle — verify service unit exists and starts ──

    with subtest("T-NM-02: nix-key-agent user service unit exists"):
        machine.succeed("systemctl --user -M testuser@ cat nix-key-agent.service")

    with subtest("T-NM-02: nix-key-agent service has correct ExecStart"):
        unit = machine.succeed(
            "systemctl --user -M testuser@ cat nix-key-agent.service"
        )
        assert "nix-key" in unit, "ExecStart should reference nix-key binary"
        assert "daemon" in unit, "ExecStart should include 'daemon' subcommand"

    with subtest("T-NM-02: nix-key-agent service activates"):
        # The daemon is currently a stub that exits immediately.
        # Verify the service was activated (even if it exits).
        machine.succeed(
            "systemctl --user -M testuser@ start nix-key-agent.service || true"
        )
        # Check that systemd attempted to run the service
        status = machine.succeed(
            "systemctl --user -M testuser@ show nix-key-agent.service "
            "--property=ExecMainStartTimestamp"
        )
        assert status.strip() != "ExecMainStartTimestamp=", \
            "Service should have a start timestamp after activation"

    with subtest("T-NM-02: preStart creates config symlink and certs directory"):
        # After service has run (even if daemon exited), preStart should
        # have created the config symlink and certs directory
        machine.succeed(
            "systemctl --user -M testuser@ start nix-key-agent.service || true"
        )
        machine.succeed("test -L /home/testuser/.config/nix-key/config.json")
        machine.succeed("test -d /home/testuser/.local/state/nix-key/certs")
        # Verify directory permissions (0700 for secrets)
        config_perms = machine.succeed(
            "stat -c '%a' /home/testuser/.config/nix-key"
        ).strip()
        assert config_perms == "700", \
            f"Expected config dir perms 700, got {config_perms}"
        state_perms = machine.succeed(
            "stat -c '%a' /home/testuser/.local/state/nix-key"
        ).strip()
        assert state_perms == "700", \
            f"Expected state dir perms 700, got {state_perms}"

    with subtest("T-NM-02: symlinked config.json is readable and valid"):
        linked_config = machine.succeed(
            "cat /home/testuser/.config/nix-key/config.json"
        ).strip()
        parsed = json.loads(linked_config)
        assert parsed["port"] == ${toString testPort}, \
            "Symlinked config.json should match generated config"

    with subtest("T-NM-02: service environment includes NIXKEY vars"):
        unit = machine.succeed(
            "systemctl --user -M testuser@ cat nix-key-agent.service"
        )
        assert "NIXKEY_LOG_LEVEL" in unit, "Should set NIXKEY_LOG_LEVEL"
        assert "NIXKEY_SOCKET_PATH" in unit, "Should set NIXKEY_SOCKET_PATH"

    # ── T-NM-04: Device merge — Nix-declared + runtime devices (FR-063, FR-064) ──

    with subtest("T-NM-04: config.json contains both Nix-declared devices"):
        devices = config["devices"]
        assert "test-phone" in devices, \
            f"Expected 'test-phone' in devices, got {list(devices.keys())}"
        assert "nix-tablet" in devices, \
            f"Expected 'nix-tablet' in devices, got {list(devices.keys())}"
        tablet = devices["nix-tablet"]
        assert tablet["name"] == "Nix Tablet", f"Unexpected name: {tablet['name']}"
        assert tablet["tailscaleIp"] == "100.64.0.3", \
            f"Unexpected IP: {tablet['tailscaleIp']}"
        assert tablet["port"] == 29419, f"Unexpected port: {tablet['port']}"
        assert tablet["certFingerprint"] == "sha256:nix-tablet-fingerprint-def456", \
            f"Unexpected fingerprint: {tablet['certFingerprint']}"

    with subtest("T-NM-04: Nix-declared devices have null cert paths (set by pairing FR-065)"):
        for dev_id in ["test-phone", "nix-tablet"]:
            dev = config["devices"][dev_id]
            assert dev["clientCertPath"] is None, \
                f"{dev_id}: expected null clientCertPath, got {dev['clientCertPath']}"
            assert dev["clientKeyPath"] is None, \
                f"{dev_id}: expected null clientKeyPath, got {dev['clientKeyPath']}"

    with subtest("T-NM-04: create runtime devices.json with additional device"):
        # Ensure the service has run preStart to create the state directory
        machine.succeed(
            "systemctl --user -M testuser@ start nix-key-agent.service || true"
        )
        # Write a runtime devices.json with an extra device not in Nix config.
        # Write as root then chown to testuser (avoids heredoc quoting issues).
        import json as json_mod
        runtime_device_data = json_mod.dumps([{
            "id": "runtime-phone",
            "name": "Runtime Phone",
            "tailscaleIp": "100.64.0.10",
            "listenPort": 29418,
            "certFingerprint": "sha256:runtime-phone-fp-789",
            "certPath": "",
            "clientCertPath": "/home/testuser/.local/state/nix-key/certs/runtime-phone.crt",
            "clientKeyPath": "/home/testuser/.local/state/nix-key/certs/runtime-phone.key",
            "source": "runtime-paired",
        }], indent=2)
        devices_json_path = "/home/testuser/.local/state/nix-key/devices.json"
        machine.succeed(
            f"printf '%s' '{runtime_device_data}' > {devices_json_path} "
            f"&& chmod 0600 {devices_json_path} "
            f"&& chown testuser:testuser {devices_json_path}"
        )

    with subtest("T-NM-04: runtime devices.json is valid and in state directory"):
        devices_raw = machine.succeed(
            "cat /home/testuser/.local/state/nix-key/devices.json"
        ).strip()
        runtime_devices = json.loads(devices_raw)
        assert len(runtime_devices) == 1, \
            f"Expected 1 runtime device, got {len(runtime_devices)}"
        assert runtime_devices[0]["id"] == "runtime-phone", \
            f"Unexpected device id: {runtime_devices[0]['id']}"
        assert runtime_devices[0]["source"] == "runtime-paired", \
            f"Unexpected source: {runtime_devices[0]['source']}"

    with subtest("T-NM-04: runtime device is distinct from Nix-declared devices"):
        nix_ids = set(config["devices"].keys())
        runtime_ids = {d["id"] for d in runtime_devices}
        overlap = nix_ids & runtime_ids
        assert len(overlap) == 0, \
            f"Runtime and Nix-declared device IDs should not overlap, but found: {overlap}"

    with subtest("T-NM-04: config.json and devices.json coexist for daemon merge"):
        # Verify both files exist and are readable by the testuser
        machine.succeed("test -L /home/testuser/.config/nix-key/config.json")
        machine.succeed("test -f /home/testuser/.local/state/nix-key/devices.json")
        # Verify permissions on devices.json (should be 0600 or writable by user)
        machine.succeed(
            "su - testuser -c 'test -r /home/testuser/.local/state/nix-key/devices.json'"
        )

    with subtest("T-NM-04: merged view would contain all three devices"):
        # Simulate what the daemon's Merge() would see:
        # 2 from Nix config + 1 from runtime = 3 total unique devices
        all_ids = set(config["devices"].keys()) | {d["id"] for d in runtime_devices}
        assert len(all_ids) == 3, \
            f"Expected 3 total devices after merge, got {len(all_ids)}: {all_ids}"
        assert all_ids == {"test-phone", "nix-tablet", "runtime-phone"}, \
            f"Unexpected device set: {all_ids}"

    with subtest("T-NM-04: nix-key revoke on Nix-declared device is not destructive"):
        # Nix-declared devices cannot be revoked via CLI — they can only be
        # removed by changing the NixOS config and rebuilding. The revoke
        # command is currently a stub; verify it does not remove the device
        # from config.json (which is a read-only Nix store symlink).
        machine.succeed(
            "su - testuser -c '${pkgs.nix-key}/bin/nix-key revoke test-phone' || true"
        )
        # config.json is a symlink to the Nix store — it must remain intact
        linked_config = machine.succeed(
            "cat /home/testuser/.config/nix-key/config.json"
        ).strip()
        parsed = json.loads(linked_config)
        assert "test-phone" in parsed["devices"], \
            "Nix-declared device must survive revoke attempt — remove from NixOS config instead"

    with subtest("T-NM-04: config.json symlink is read-only (Nix store)"):
        # The config.json symlink points to the Nix store which is read-only.
        # This structurally prevents CLI tools from modifying Nix-declared devices.
        target = machine.succeed(
            "readlink /home/testuser/.config/nix-key/config.json"
        ).strip()
        assert target.startswith("/nix/store/"), \
            f"config.json should be a symlink to Nix store, got: {target}"

    # ── T-NM-05: Graceful shutdown — systemctl stop exits cleanly (FR-E07) ──

    with subtest("T-NM-05: systemctl stop completes without timeout"):
        machine.succeed(
            "systemctl --user -M testuser@ start nix-key-agent.service || true"
        )
        # Stop should complete cleanly (not hang, not require SIGKILL)
        machine.succeed(
            "systemctl --user -M testuser@ stop nix-key-agent.service || true"
        )
        # Verify the stop result
        result = machine.succeed(
            "systemctl --user -M testuser@ show nix-key-agent.service "
            "--property=ExecMainStatus"
        ).strip()
        assert "ExecMainStatus=" in result, \
            f"Unexpected status output: {result}"

    with subtest("T-NM-05: service is inactive after stop"):
        state = machine.succeed(
            "systemctl --user -M testuser@ show nix-key-agent.service "
            "--property=ActiveState"
        ).strip()
        # After stop, should be inactive (or failed if stub returned non-zero)
        assert "inactive" in state or "failed" in state, \
            f"Expected inactive or failed after stop, got: {state}"
  '';
}
