# NixOS VM test for the nix-key pairing flow over a real Tailnet (headscale).
# Validates: T-E2E-02 (pairing E2E), Story 2, SC-002.
#
# Test topology:
#   headscale (on host node) -> host tailscaled + nix-key pair
#                             -> phone tailscaled (simulates phone)
#   Phone node uses curl to POST to host's pairing endpoint.
{ pkgs, nixKeyModule }:
let
  # Shared headscale config
  headscaleDomain = "headscale.test";
  headscalePort = 8080;

  # The phonesim package
  phonesimPkg = pkgs.phonesim;

in
{
  name = "nix-key-pairing";

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
        secrets.ageKeyFile = "/tmp/test-age-identity.txt";
      };

      # Minimal static DERP map for headscale (offline VM, no internet)
      environment.etc."headscale/derp.yaml".text = ''
        regions:
          900:
            regionid: 900
            regioncode: test
            regionname: "Test DERP"
            nodes:
              - name: test-derp
                regionid: 900
                hostname: 127.0.0.1
                stunport: -1
                derpport: 0
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
          };
          # Disable TLS for test simplicity
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

      # Open firewall for headscale and pairing
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
        pkgs.age
        pkgs.headscale
        pkgs.jq
        pkgs.curl
      ];
    };

  nodes.phone =
    { config, lib, ... }:
    {
      # Tailscale client on phone
      services.tailscale.enable = true;

      # DNS resolution for headscale on host
      networking.extraHosts = ''
        192.168.1.1 ${headscaleDomain}
      '';

      # Open firewall
      networking.firewall.enable = false;

      environment.systemPackages = [
        phonesimPkg
        pkgs.jq
        pkgs.curl
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
        # Create a user (namespace) for our test nodes
        host.succeed("headscale users create nixkey-test")

        # Retrieve numeric user ID (headscale v0.28+ requires uint, not username)
        user_id = host.succeed(
            "headscale users list -o json | jq -r '.[0].id'"
        ).strip()

        # Create pre-auth keys for host and phone
        host_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()
        phone_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()

        # Store keys for later use
        host.succeed(f"echo '{host_key}' > /tmp/host-ts-key")
        host.succeed(f"echo '{phone_key}' > /tmp/phone-ts-key")

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

    # ── Phase 3: Verify Tailnet connectivity ──

    with subtest("verify both nodes on tailnet"):
        # Get Tailscale IPs
        host_ts_ip = host.succeed("tailscale ip -4").strip()
        phone_ts_ip = phone.succeed("tailscale ip -4").strip()
        assert host_ts_ip.startswith("100."), f"Host IP should be in 100.x range, got: {host_ts_ip}"
        assert phone_ts_ip.startswith("100."), f"Phone IP should be in 100.x range, got: {phone_ts_ip}"

        # Verify connectivity
        host.wait_until_succeeds(f"ping -c 1 -W 5 {phone_ts_ip}", timeout=30)
        phone.wait_until_succeeds(f"ping -c 1 -W 5 {host_ts_ip}", timeout=30)

    # ── Phase 4: Run nix-key pair with auto-confirm ──

    with subtest("prepare pairing environment"):
        # Create state directories for testuser
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key")
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key/certs")
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.config/nix-key")

    with subtest("start nix-key pair in background"):
        # Generate a self-signed server cert for the phone simulator
        phone.succeed(
            "openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 "
            "-keyout /tmp/phone-server-key.pem -out /tmp/phone-server-cert.pem "
            "-days 1 -nodes -subj '/CN=phonesim' 2>/dev/null"
        )
        phone_cert = phone.succeed("cat /tmp/phone-server-cert.pem").strip()

        # Run nix-key pair on host as testuser, with:
        #   --interface tailscale0 (real Tailscale interface)
        #   --pair-info-file to export connection info for the phone
        #   --hostname test-host
        #   auto-confirm via 'yes y' piped to stdin
        #   --age-key-file pointing to a test identity
        #   --devices-path and --certs-dir in testuser's home
        host.succeed(
            "su - testuser -c '"
            "yes y | ${pkgs.nix-key}/bin/nix-key pair "
            "--interface tailscale0 "
            "--hostname test-host "
            "--pair-info-file /tmp/pair-info.json "
            "--age-key-file /home/testuser/.local/state/nix-key/age-identity.txt "
            "--devices-path /home/testuser/.local/state/nix-key/devices.json "
            "--certs-dir /home/testuser/.local/state/nix-key/certs "
            ">/tmp/pair-output.log 2>&1 &'"
        )

        # Wait for pair-info.json to appear (pairing server is ready)
        host.wait_until_succeeds("test -f /tmp/pair-info.json", timeout=30)

    with subtest("read pairing info"):
        pair_info_raw = host.succeed("cat /tmp/pair-info.json").strip()
        pair_info = json.loads(pair_info_raw)
        pair_host = pair_info["Host"]
        pair_port = pair_info["Port"]
        pair_token = pair_info["Token"]
        pair_cert = pair_info["Cert"]
        assert pair_host == host_ts_ip, \
            f"Pairing host should be Tailscale IP {host_ts_ip}, got {pair_host}"
        assert pair_port > 0, f"Pairing port should be positive, got {pair_port}"
        assert len(pair_token) > 0, "Pairing token should not be empty"

    # ── Phase 5: Phone connects to pairing endpoint ──

    with subtest("phone posts to pairing endpoint"):
        # Build the JSON payload the phone would send.
        # Write to file first to avoid shell quoting issues with PEM certs.
        import json as json_mod
        pairing_request = json_mod.dumps({
            "phoneName": "Test Phone Sim",
            "tailscaleIp": phone_ts_ip,
            "listenPort": 50051,
            "serverCert": phone_cert,
            "token": pair_token,
        })
        phone.succeed(f"cat > /tmp/pairing-request.json << 'PAIREOF'\n{pairing_request}\nPAIREOF")

        # POST to the host's pairing endpoint over HTTPS (self-signed cert, skip verification)
        phone.succeed(
            f"curl -sk --max-time 30 "
            f"-X POST "
            f"-H 'Content-Type: application/json' "
            f"-d @/tmp/pairing-request.json "
            f"https://{pair_host}:{pair_port}/pair "
            f"-o /tmp/pair-response.json"
        )

        # Verify the response
        pair_response_raw = phone.succeed("cat /tmp/pair-response.json").strip()
        pair_response = json.loads(pair_response_raw)
        assert pair_response["status"] == "approved", \
            f"Expected pairing status 'approved', got: {pair_response['status']}"
        assert pair_response["hostName"] == "test-host", \
            f"Expected hostName 'test-host', got: {pair_response.get('hostName')}"
        assert "hostClientCert" in pair_response and len(pair_response["hostClientCert"]) > 0, \
            "Response should include a host client certificate"

    # ── Phase 6: Verify device registered ──

    with subtest("wait for pairing to complete on host"):
        # Give nix-key pair a moment to process the result (encrypt certs, save device)
        host.wait_until_succeeds(
            "test -f /home/testuser/.local/state/nix-key/devices.json",
            timeout=30,
        )

    with subtest("verify device registered in devices.json"):
        devices_raw = host.succeed(
            "cat /home/testuser/.local/state/nix-key/devices.json"
        ).strip()
        devices = json.loads(devices_raw)

        # devices.json is a JSON array of device objects
        assert isinstance(devices, list), \
            f"Expected devices to be a list, got {type(devices).__name__}"
        assert len(devices) >= 1, \
            f"Expected at least 1 device, got {len(devices)}"

        # Find the device that was just paired
        paired_device = None
        for d in devices:
            if d.get("name") == "Test Phone Sim":
                paired_device = d
                break
        assert paired_device is not None, \
            f"Expected device 'Test Phone Sim' in devices.json, got: {[d.get('name') for d in devices]}"

        # Verify device fields
        assert paired_device["tailscaleIp"] == phone_ts_ip, \
            f"Expected tailscaleIp {phone_ts_ip}, got {paired_device['tailscaleIp']}"
        assert paired_device["listenPort"] == 50051, \
            f"Expected listenPort 50051, got {paired_device['listenPort']}"
        assert paired_device.get("source") == "runtime-paired", \
            f"Expected source 'runtime-paired', got {paired_device.get('source')}"
        assert paired_device.get("certFingerprint", "") != "", \
            "Expected non-empty certFingerprint"

    with subtest("verify certs stored on host"):
        # The cert directory is named by the first 16 chars of the cert fingerprint
        fp = paired_device["certFingerprint"]
        cert_dir = f"/home/testuser/.local/state/nix-key/certs/{fp[:16]}"
        host.succeed(f"test -d {cert_dir}")
        host.succeed(f"test -f {cert_dir}/phone-server-cert.pem")
        host.succeed(f"test -f {cert_dir}/host-client-cert.pem")
        host.succeed(f"test -f {cert_dir}/host-client-key.pem.age")

        # Verify the phone server cert matches what the phone sent
        stored_cert = host.succeed(f"cat {cert_dir}/phone-server-cert.pem").strip()
        assert stored_cert == phone_cert, \
            "Stored phone server cert should match what the phone sent"

    with subtest("verify host client key is age-encrypted"):
        # The encrypted key file should NOT be valid PEM (it's age-encrypted)
        encrypted_key = host.succeed(
            f"cat {cert_dir}/host-client-key.pem.age"
        ).strip()
        assert "age-encryption.org" in encrypted_key, \
            "Encrypted key should contain age header"
        assert "BEGIN EC PRIVATE KEY" not in encrypted_key, \
            "Encrypted key should not contain plaintext PEM"

    with subtest("verify token replay is rejected"):
        # Try to pair again with the same token — should be rejected.
        # The server may have shut down after the first successful pairing,
        # in which case the connection will fail (which is also acceptable).
        # If it's still running, the token should be rejected (401).
        replay_result = phone.succeed(
            f"curl -sk --max-time 10 -w '%{{http_code}}' "
            f"-X POST "
            f"-H 'Content-Type: application/json' "
            f"-d @/tmp/pairing-request.json "
            f"https://{pair_host}:{pair_port}/pair "
            f"-o /dev/null || true"
        ).strip()
        if "000" not in replay_result:
            assert replay_result == "401", \
                f"Expected HTTP 401 for token replay, got: {replay_result}"
  '';
}
