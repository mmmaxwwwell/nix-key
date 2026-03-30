# NixOS VM test for the nix-key signing flow over a real Tailnet (headscale).
# Validates: T-E2E-01 (signing E2E), Story 1, SC-001, SC-006.
#
# Test topology:
#   headscale (on host node) → host tailscaled + nix-key daemon
#                             → phone tailscaled + phonesim (plain gRPC on Tailscale)
#
# Scenarios:
#   1. Success: phonesim auto-approve → ssh-add -L lists keys → sign succeeds
#   2. Timeout: phonesim with 60s sign delay, signTimeout=5s → SSH_AGENT_FAILURE
#   3. Denial: phonesim in deny mode → sign fails
{ pkgs, nixKeyModule }:
let
  headscaleDomain = "headscale.test";
  headscalePort = 8080;

  phonesimPkg = pkgs.phonesim;
  phonesimPort = 50051;

  # Agent socket path for direct daemon invocation (not via systemd)
  agentSocketPath = "/tmp/nix-key-test/agent.sock";
in
{
  name = "nix-key-signing";

  nodes.host =
    { config, lib, ... }:
    {
      imports = [ nixKeyModule ];

      # Enable nix-key with test configuration
      services.nix-key = {
        enable = true;
        package = pkgs.nix-key;
        tailscaleInterface = "tailscale0";
        logLevel = "debug";
        signTimeout = 30;
        connectionTimeout = 10;
        socketPath = agentSocketPath;
        secrets.ageKeyFile = "/tmp/test-age-identity.txt";
      };

      # Minimal static DERP map so headscale startup validation passes
      # (non-empty map required). The embedded DERP server provides actual relay.
      environment.etc."headscale/derp.yaml".text = ''
        regions:
          900:
            regionid: 900
            regioncode: test
            regionname: "Test DERP"
            nodes:
              - name: test-derp
                regionid: 900
                hostname: 192.168.1.1
                ipv4: 192.168.1.1
                stunport: -1
                derpport: 8080
      '';

      # Headscale server on this node
      services.headscale = {
        enable = true;
        settings = {
          server_url = "http://${headscaleDomain}:${toString headscalePort}";
          listen_addr = "0.0.0.0:${toString headscalePort}";
          ip_prefixes = [
            "100.64.0.0/10"
          ];
          dns = {
            base_domain = "test.ts";
            nameservers.global = [ "127.0.0.1" ];
            magic_dns = false;
          };
          derp = {
            urls = [ ];
            paths = [ "/etc/headscale/derp.yaml" ];
            auto_update_enabled = false;
            update_frequency = "1h";
            server = {
              enabled = true;
              region_id = 999;
              region_code = "headscale";
              region_name = "Headscale Embedded DERP";
              stun_listen_addr = "0.0.0.0:3478";
              ipv4 = "192.168.1.1";
              automatically_add_embedded_derp_region = true;
            };
          };
          tls_cert_path = null;
          tls_key_path = null;
        };
      };

      # Tailscale client on host
      services.tailscale.enable = true;

      # DNS resolution for headscale
      networking.extraHosts = ''
        127.0.0.1 ${headscaleDomain}
      '';

      # Open firewall for headscale and agent
      networking.firewall.enable = false;

      # Regular user for nix-key
      users.users.testuser = {
        isNormalUser = true;
        uid = 1000;
        group = "testuser";
      };
      users.groups.testuser = { };

      systemd.tmpfiles.rules = [
        "f /var/lib/systemd/linger/testuser - - - -"
      ];

      environment.systemPackages = [
        pkgs.nix-key
        pkgs.openssh
        pkgs.age
        pkgs.headscale
        pkgs.jq
      ];
    };

  nodes.phone =
    { config, lib, ... }:
    {
      # Tailscale client on phone node
      services.tailscale.enable = true;

      # DNS resolution for headscale on host
      networking.extraHosts = ''
        192.168.1.1 ${headscaleDomain}
      '';

      # Open firewall for phonesim gRPC
      networking.firewall.enable = false;

      environment.systemPackages = [
        phonesimPkg
        pkgs.jq
      ];
    };

  testScript = ''
    import json

    start_all()

    # ── Phase 1: Headscale setup ──

    with subtest("headscale starts"):
        # Fail fast if headscale crash-loops instead of waiting 900s default timeout.
        host.succeed(
            "for i in $(seq 1 30); do "
            "  state=$(systemctl is-active headscale.service || true); "
            "  if [ \"$state\" = \"active\" ]; then exit 0; fi; "
            "  if [ \"$state\" = \"failed\" ]; then "
            "    echo 'headscale.service entered failed state:'; "
            "    journalctl -u headscale.service --no-pager -n 20; "
            "    exit 1; "
            "  fi; "
            "  sleep 2; "
            "done; "
            "echo 'headscale.service did not become active within 60s'; "
            "journalctl -u headscale.service --no-pager -n 20; "
            "exit 1"
        )
        host.wait_for_open_port(${toString headscalePort}, timeout=30)

    with subtest("create headscale user and pre-auth keys"):
        host.succeed("headscale users create nixkey-test")

        # Retrieve numeric user ID (headscale v0.28+ requires uint, not username)
        user_id = host.succeed(
            "headscale users list -o json | jq -r '.[0].id'"
        ).strip()

        host_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()
        phone_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()

    # ── Phase 2: Join Tailnet ──

    with subtest("host joins tailnet"):
        host.wait_for_unit("tailscaled.service")
        host.succeed(
            f"tailscale up --login-server http://${headscaleDomain}:${toString headscalePort} "
            f"--auth-key {host_key} --hostname test-host"
        )

    with subtest("phone joins tailnet"):
        phone.wait_for_unit("tailscaled.service")
        phone.succeed(
            f"tailscale up --login-server http://${headscaleDomain}:${toString headscalePort} "
            f"--auth-key {phone_key} --hostname test-phone"
        )

    with subtest("verify tailnet connectivity"):
        host_ts_ip = host.succeed("tailscale ip -4").strip()
        phone_ts_ip = phone.succeed("tailscale ip -4").strip()
        assert host_ts_ip.startswith("100."), f"Host IP should be in 100.x range, got: {host_ts_ip}"
        assert phone_ts_ip.startswith("100."), f"Phone IP should be in 100.x range, got: {phone_ts_ip}"
        host.wait_until_succeeds(f"ping -c 1 -W 5 {phone_ts_ip}", timeout=120)
        phone.wait_until_succeeds(f"ping -c 1 -W 5 {host_ts_ip}", timeout=120)

    # ── Phase 3: Start phonesim in auto-approve mode ──
    # phonesim uses -plain-listen to bind on all interfaces; traffic arrives
    # via the Tailscale overlay (phone's system tailscaled handles routing).

    with subtest("start phonesim auto-approve"):
        phone.succeed(
            "phonesim -plain-listen 0.0.0.0:${toString phonesimPort} "
            ">/tmp/phonesim.log 2>&1 &"
        )
        phone.wait_for_open_port(${toString phonesimPort})

    # ── Phase 4: Create pre-paired state on host ──
    # Write devices.json with the phonesim's Tailscale IP and port so the
    # daemon knows how to reach it. This simulates a device that was
    # previously paired via `nix-key pair`.

    with subtest("create pre-paired state"):
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key")
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key/certs")
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.config/nix-key")
        host.succeed("install -d -m 0700 -o testuser -g testuser /tmp/nix-key-test")

        import json as json_mod
        devices_data = json_mod.dumps([{
            "id": "phonesim",
            "name": "Phone Simulator",
            "tailscaleIp": phone_ts_ip,
            "listenPort": ${toString phonesimPort},
            "certFingerprint": "sha256:phonesim-test-fingerprint",
            "certPath": "",
            "clientCertPath": "",
            "clientKeyPath": "",
            "source": "runtime-paired",
        }], indent=2)
        devices_path = "/home/testuser/.local/state/nix-key/devices.json"
        host.succeed(
            f"printf '%s' '{devices_data}' > {devices_path} "
            f"&& chmod 0600 {devices_path} "
            f"&& chown testuser:testuser {devices_path}"
        )

        # Symlink the Nix-generated config.json into the user's config dir
        config_store_path = host.succeed(
            "find /nix/store -maxdepth 1 -name '*nix-key-config.json' | head -1"
        ).strip()
        host.succeed(
            f"ln -sf {config_store_path} /home/testuser/.config/nix-key/config.json "
            f"&& chown -h testuser:testuser /home/testuser/.config/nix-key/config.json"
        )

    # ── Phase 5: Start nix-key daemon ──

    with subtest("start nix-key daemon"):
        # Stop the auto-started systemd user service (it booted with no devices)
        host.succeed("systemctl --user -M testuser@ stop nix-key-agent.service || true")
        host.succeed(
            "su - testuser -c '"
            "NIXKEY_CONTROL_SOCKET_PATH=/tmp/nix-key-test/control.sock "
            "${pkgs.nix-key}/bin/nix-key daemon "
            "--config /home/testuser/.config/nix-key/config.json "
            ">/tmp/nix-key-daemon.log 2>&1 &'"
        )
        # Wait for the SSH agent socket to appear
        host.wait_until_succeeds(
            "test -S ${agentSocketPath}",
            timeout=30,
        )

    # ── Phase 6: Test ssh-add -L lists phonesim keys ──

    with subtest("ssh-add -L lists phonesim keys"):
        # Capture daemon log for diagnostics if listing fails
        daemon_log = host.succeed("cat /tmp/nix-key-daemon.log 2>/dev/null || true")
        keys_output = host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L'"
        ).strip()
        # phonesim generates Ed25519 + ECDSA keys
        assert "ssh-ed25519" in keys_output or "ecdsa-sha2-nistp256" in keys_output, \
            f"Expected SSH key types in output, got: {keys_output}"
        # Save the first key for signing tests
        host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L' "
            "| head -1 > /tmp/sign-key.pub"
        )
        key_line = host.succeed("cat /tmp/sign-key.pub").strip()
        assert len(key_line) > 0, "Sign key file should not be empty"

    # ── Phase 7: Test SSH sign operation succeeds ──

    with subtest("SSH sign operation with auto-approve succeeds"):
        host.succeed("echo 'test data for signing' > /tmp/test-data.txt")
        host.succeed(
            "su - testuser -c '"
            "SSH_AUTH_SOCK=${agentSocketPath} "
            "ssh-keygen -Y sign -f /tmp/sign-key.pub -n test "
            "< /tmp/test-data.txt > /tmp/test-signature'"
        )
        sig_output = host.succeed("cat /tmp/test-signature").strip()
        assert "BEGIN SSH SIGNATURE" in sig_output, \
            f"Expected SSH signature block, got: {sig_output[:200]}"

    # ── Phase 8: Test timeout (phonesim with 60s delay, signTimeout=5s) ──

    with subtest("stop phonesim for timeout test"):
        phone.execute("pkill phonesim")
        phone.wait_until_fails("pgrep phonesim", timeout=10)

    with subtest("start phonesim with 60s sign delay"):
        phone.succeed(
            "phonesim -plain-listen 0.0.0.0:${toString phonesimPort} -sign-delay 60s "
            ">/tmp/phonesim-timeout.log 2>&1 &"
        )
        phone.wait_for_open_port(${toString phonesimPort})

    with subtest("restart daemon with signTimeout=5s for timeout test"):
        host.succeed("su - testuser -c 'pkill -f \"nix-key daemon\"' || true")
        host.wait_until_fails(
            "test -S ${agentSocketPath}",
            timeout=10,
        )
        host.succeed(
            "su - testuser -c '"
            "NIXKEY_SIGN_TIMEOUT=5 "
            "NIXKEY_CONTROL_SOCKET_PATH=/tmp/nix-key-test/control.sock "
            "${pkgs.nix-key}/bin/nix-key daemon "
            "--config /home/testuser/.config/nix-key/config.json "
            ">/tmp/nix-key-daemon-timeout.log 2>&1 &'"
        )
        host.wait_until_succeeds(
            "test -S ${agentSocketPath}",
            timeout=30,
        )

    with subtest("sign with timeout yields SSH_AGENT_FAILURE"):
        # Listing should still work (no delay on ListKeys)
        host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L' "
            "| head -1 > /tmp/sign-key-timeout.pub"
        )
        # Sign should fail: phonesim delays 60s, daemon times out at 5s
        host.fail(
            "su - testuser -c '"
            "SSH_AUTH_SOCK=${agentSocketPath} "
            "ssh-keygen -Y sign -f /tmp/sign-key-timeout.pub -n test "
            "< /tmp/test-data.txt > /dev/null 2>&1'"
        )

    # ── Phase 9: Test denial (phonesim in deny mode) ──

    with subtest("stop phonesim for denial test"):
        phone.execute("pkill -9 phonesim")
        phone.wait_until_fails("pgrep phonesim", timeout=10)

    with subtest("start phonesim in deny mode"):
        phone.succeed(
            "phonesim -plain-listen 0.0.0.0:${toString phonesimPort} -deny-sign "
            ">/tmp/phonesim-deny.log 2>&1 &"
        )
        phone.wait_for_open_port(${toString phonesimPort})

    with subtest("restart daemon for denial test"):
        host.succeed("su - testuser -c 'pkill -f \"nix-key daemon\"' || true")
        host.wait_until_fails(
            "test -S ${agentSocketPath}",
            timeout=10,
        )
        host.succeed(
            "su - testuser -c '"
            "NIXKEY_CONTROL_SOCKET_PATH=/tmp/nix-key-test/control.sock "
            "${pkgs.nix-key}/bin/nix-key daemon "
            "--config /home/testuser/.config/nix-key/config.json "
            ">/tmp/nix-key-daemon-deny.log 2>&1 &'"
        )
        host.wait_until_succeeds(
            "test -S ${agentSocketPath}",
            timeout=30,
        )

    with subtest("sign with denial yields failure"):
        # Listing should work (deny-sign only affects signing, not listing)
        host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L' "
            "| head -1 > /tmp/sign-key-deny.pub"
        )
        # Sign should fail: phonesim denies the sign request
        host.fail(
            "su - testuser -c '"
            "SSH_AUTH_SOCK=${agentSocketPath} "
            "ssh-keygen -Y sign -f /tmp/sign-key-deny.pub -n test "
            "< /tmp/test-data.txt > /dev/null 2>&1'"
        )
  '';
}
