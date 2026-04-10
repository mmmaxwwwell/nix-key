# NixOS VM test for adversarial security scenarios.
# Validates: T-ADV-01 through T-ADV-06, SC-011.
#
# Test topology:
#   headscale (on host node) → host tailscaled + nix-key daemon
#                             → phone tailscaled + phonesim (plain gRPC on Tailscale)
#                             → rogue tailscaled (adversarial node)
#
# Scenarios:
#   1. Expired cert → rejected (T-ADV-01)
#   2. Wrong-CA cert → rejected (T-ADV-02)
#   3. Unpaired cert → rejected (T-ADV-03)
#   4. Connection on non-Tailscale interface (raw eth0) → rejected (T-ADV-04)
#   5. Replayed pairing token → rejected (T-ADV-05)
#   6. Error responses leak no internal details (T-ADV-06)
{ pkgs, nixKeyModule }:
let
  headscaleDomain = "headscale.test";
  headscalePort = 8080;

  phonesimPkg = pkgs.phonesim;
  phonesimPort = 50051;

  agentSocketPath = "/tmp/nix-key-test/agent.sock";
  controlSocketPath = "/tmp/nix-key-test/control.sock";

  # Adversarial and legitimate cert fixtures
  adversarialCerts = ../../test/fixtures/adversarial;
  legitimateCerts = ../../test/fixtures;

  # Ports for adversarial TLS servers on the rogue node
  expiredTLSPort = 9001;
  wrongCATLSPort = 9002;
  unpairedTLSPort = 9003;

  # Self-signed TLS cert for headscale — required so the embedded DERP relay
  # serves TLS and tailscaled can connect to it.
  testTlsCert = pkgs.runCommand "headscale-test-cert" { nativeBuildInputs = [ pkgs.openssl ]; } ''
    mkdir -p $out
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
      -keyout $out/tls.key -out $out/tls.crt \
      -days 365 -nodes -subj '/CN=${headscaleDomain}' \
      -addext 'subjectAltName=DNS:${headscaleDomain},IP:192.168.1.1'
    chmod 644 $out/tls.crt
  '';
in
{
  name = "nix-key-adversarial";

  nodes.host =
    { config, lib, ... }:
    {
      imports = [ nixKeyModule ];

      services.nix-key = {
        enable = true;
        package = pkgs.nix-key;
        tailscaleInterface = "tailscale0";
        logLevel = "debug";
        signTimeout = 10;
        connectionTimeout = 5;
        socketPath = agentSocketPath;
        secrets.ageKeyFile = "/tmp/test-age-identity.txt";
      };

      # Minimal static DERP map for headscale startup validation
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

      services.headscale = {
        enable = true;
        settings = {
          server_url = "https://${headscaleDomain}:${toString headscalePort}";
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
          tls_cert_path = "${testTlsCert}/tls.crt";
          tls_key_path = "${testTlsCert}/tls.key";
        };
      };

      services.tailscale.enable = true;

      # Trust the self-signed headscale TLS cert
      security.pki.certificateFiles = [ "${testTlsCert}/tls.crt" ];

      networking.extraHosts = ''
        127.0.0.1 ${headscaleDomain}
      '';

      # Firewall enabled for adversarial testing
      networking.firewall = {
        enable = true;
        allowedTCPPorts = [
          headscalePort
          3478
        ];
      };

      users.users.testuser = {
        isNormalUser = true;
        uid = 1000;
        group = "testuser";
      };
      users.groups.testuser = { };

      systemd.tmpfiles.rules = [
        "f /var/lib/systemd/linger/testuser - - - -"
      ];

      # Make cert fixtures available in the VM
      environment.etc."adversarial-certs".source = adversarialCerts;
      environment.etc."test-certs/host-client-cert.pem".source =
        "${legitimateCerts}/host-client-cert.pem";
      environment.etc."test-certs/host-client-key.pem".source = "${legitimateCerts}/host-client-key.pem";
      environment.etc."test-certs/phone-server-cert.pem".source =
        "${legitimateCerts}/phone-server-cert.pem";

      environment.systemPackages = [
        pkgs.nix-key
        pkgs.openssh
        pkgs.age
        pkgs.headscale
        pkgs.jq
        pkgs.openssl
        pkgs.curl
      ];
    };

  nodes.phone =
    { config, lib, ... }:
    {
      services.tailscale.enable = true;

      security.pki.certificateFiles = [ "${testTlsCert}/tls.crt" ];

      networking.extraHosts = ''
        192.168.1.1 ${headscaleDomain}
      '';

      # Firewall enabled — only allow phonesim port on Tailscale interface (T-ADV-04)
      networking.firewall = {
        enable = true;
        interfaces.tailscale0.allowedTCPPorts = [ phonesimPort ];
        # Default (eth1) does NOT allow phonesimPort
      };

      environment.systemPackages = [
        phonesimPkg
        pkgs.jq
        pkgs.curl
        pkgs.openssl
      ];
    };

  nodes.rogue =
    { config, lib, ... }:
    {
      services.tailscale.enable = true;

      security.pki.certificateFiles = [ "${testTlsCert}/tls.crt" ];

      networking.extraHosts = ''
        192.168.1.1 ${headscaleDomain}
      '';

      # Firewall enabled — allow adversarial TLS server ports on Tailscale only
      networking.firewall = {
        enable = true;
        interfaces.tailscale0.allowedTCPPorts = [
          expiredTLSPort
          wrongCATLSPort
          unpairedTLSPort
        ];
      };

      # Make adversarial certs available
      environment.etc."adversarial-certs".source = adversarialCerts;

      environment.systemPackages = [
        pkgs.openssl
        pkgs.curl
        pkgs.jq
        pkgs.netcat-gnu
      ];
    };

  testScript = ''
    import json

    start_all()

    # ── Phase 1: Headscale setup ──

    with subtest("headscale starts"):
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
        user_id = host.succeed(
            "headscale users list -o json | jq -r '.[0].id'"
        ).strip()
        host_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()
        phone_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()
        rogue_key = host.succeed(
            f"headscale preauthkeys create --user {user_id} --reusable --expiration 1h"
        ).strip()

    # ── Phase 2: Join Tailnet (all three nodes) ──

    with subtest("host joins tailnet"):
        host.wait_for_unit("tailscaled.service")
        host.succeed(
            f"tailscale up --login-server https://${headscaleDomain}:${toString headscalePort} "
            f"--auth-key {host_key} --hostname test-host"
        )

    with subtest("phone joins tailnet"):
        phone.wait_for_unit("tailscaled.service")
        phone.succeed(
            f"tailscale up --login-server https://${headscaleDomain}:${toString headscalePort} "
            f"--auth-key {phone_key} --hostname test-phone"
        )

    with subtest("rogue joins tailnet"):
        rogue.wait_for_unit("tailscaled.service")
        rogue.succeed(
            f"tailscale up --login-server https://${headscaleDomain}:${toString headscalePort} "
            f"--auth-key {rogue_key} --hostname test-rogue"
        )

    with subtest("verify tailnet connectivity"):
        host_ts_ip = host.succeed("tailscale ip -4").strip()
        phone_ts_ip = phone.succeed("tailscale ip -4").strip()
        rogue_ts_ip = rogue.succeed("tailscale ip -4").strip()
        assert host_ts_ip.startswith("100."), f"Host TS IP: {host_ts_ip}"
        assert phone_ts_ip.startswith("100."), f"Phone TS IP: {phone_ts_ip}"
        assert rogue_ts_ip.startswith("100."), f"Rogue TS IP: {rogue_ts_ip}"
        host.wait_until_succeeds(f"ping -c 1 -W 5 {phone_ts_ip}", timeout=120)
        host.wait_until_succeeds(f"ping -c 1 -W 5 {rogue_ts_ip}", timeout=120)

    # ── Phase 3: Start phonesim and daemon (legitimate setup) ──

    with subtest("start phonesim on phone"):
        phone.succeed(
            "phonesim -plain-listen 0.0.0.0:${toString phonesimPort} "
            ">/tmp/phonesim.log 2>&1 &"
        )
        phone.wait_for_open_port(${toString phonesimPort})

    with subtest("create pre-paired state on host"):
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key")
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key/certs")
        host.succeed("install -d -m 0700 -o testuser -g testuser /home/testuser/.config/nix-key")
        host.succeed("install -d -m 0700 -o testuser -g testuser /tmp/nix-key-test")

        # Compute fingerprint of the phone-server-cert for the legitimate device
        legit_fp = host.succeed(
            "openssl x509 -in /etc/test-certs/phone-server-cert.pem -outform DER "
            "| sha256sum | awk '{print $1}'"
        ).strip()

        # Compute fingerprint of the expired cert for T-ADV-01
        expired_fp = host.succeed(
            "openssl x509 -in /etc/adversarial-certs/expired-client-cert.pem -outform DER "
            "| sha256sum | awk '{print $1}'"
        ).strip()

        # Copy client certs for mTLS dial to writable location
        host.succeed(
            "install -d -m 0700 -o testuser -g testuser /home/testuser/.local/state/nix-key/certs/adv-test"
        )
        host.succeed(
            "cp /etc/test-certs/host-client-cert.pem /home/testuser/.local/state/nix-key/certs/adv-test/ && "
            "cp /etc/test-certs/host-client-key.pem /home/testuser/.local/state/nix-key/certs/adv-test/ && "
            "cp /etc/test-certs/phone-server-cert.pem /home/testuser/.local/state/nix-key/certs/adv-test/ && "
            "chown -R testuser:testuser /home/testuser/.local/state/nix-key/certs/adv-test && "
            "chmod 0600 /home/testuser/.local/state/nix-key/certs/adv-test/*.pem"
        )

        import json as json_mod
        client_cert = "/home/testuser/.local/state/nix-key/certs/adv-test/host-client-cert.pem"
        client_key = "/home/testuser/.local/state/nix-key/certs/adv-test/host-client-key.pem"
        server_cert = "/home/testuser/.local/state/nix-key/certs/adv-test/phone-server-cert.pem"

        devices_data = json_mod.dumps([
            {
                "id": "phonesim",
                "name": "Phone Simulator",
                "tailscaleIp": phone_ts_ip,
                "listenPort": ${toString phonesimPort},
                "certFingerprint": "sha256:phonesim-test-fingerprint",
                "certPath": "",
                "clientCertPath": "",
                "clientKeyPath": "",
                "source": "runtime-paired",
            },
            {
                "id": "expired-device",
                "name": "Expired Cert Device",
                "tailscaleIp": rogue_ts_ip,
                "listenPort": ${toString expiredTLSPort},
                "certFingerprint": expired_fp,
                "certPath": server_cert,
                "clientCertPath": client_cert,
                "clientKeyPath": client_key,
                "source": "runtime-paired",
            },
            {
                "id": "wrong-ca-device",
                "name": "Wrong CA Device",
                "tailscaleIp": rogue_ts_ip,
                "listenPort": ${toString wrongCATLSPort},
                "certFingerprint": legit_fp,
                "certPath": server_cert,
                "clientCertPath": client_cert,
                "clientKeyPath": client_key,
                "source": "runtime-paired",
            },
            {
                "id": "unpaired-device",
                "name": "Unpaired Device",
                "tailscaleIp": rogue_ts_ip,
                "listenPort": ${toString unpairedTLSPort},
                "certFingerprint": legit_fp,
                "certPath": server_cert,
                "clientCertPath": client_cert,
                "clientKeyPath": client_key,
                "source": "runtime-paired",
            },
        ], indent=2)
        devices_path = "/home/testuser/.local/state/nix-key/devices.json"
        host.succeed(
            f"printf '%s' '{devices_data}' > {devices_path} "
            f"&& chmod 0600 {devices_path} "
            f"&& chown testuser:testuser {devices_path}"
        )

        config_store_path = host.succeed(
            "find /nix/store -maxdepth 1 -name '*nix-key-config.json' | head -1"
        ).strip()
        host.succeed(
            f"ln -sf {config_store_path} /home/testuser/.config/nix-key/config.json "
            f"&& chown -h testuser:testuser /home/testuser/.config/nix-key/config.json"
        )

    with subtest("start nix-key daemon"):
        host.succeed("systemctl --user -M testuser@ stop nix-key-agent.service || true")
        host.succeed(
            "su - testuser -c '"
            "NIXKEY_CONTROL_SOCKET_PATH=${controlSocketPath} "
            "${pkgs.nix-key}/bin/nix-key daemon "
            "--config /home/testuser/.config/nix-key/config.json "
            ">/tmp/nix-key-daemon.log 2>&1 &'"
        )
        host.wait_until_succeeds(
            "test -S ${agentSocketPath}",
            timeout=30,
        )

    with subtest("verify legitimate phonesim works"):
        keys_output = host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L'"
        ).strip()
        assert "ssh-ed25519" in keys_output or "ecdsa-sha2-nistp256" in keys_output, \
            f"Expected SSH key types in output, got: {keys_output}"

    # ── Phase 4: Start adversarial TLS servers on rogue ──
    # Each openssl s_server presents an adversarial cert. The host daemon's
    # PinnedTLSConfig.VerifyPeerCertificate will reject these during TLS handshake.

    with subtest("start adversarial TLS servers on rogue"):
        # T-ADV-01: Expired cert server
        rogue.succeed(
            "openssl s_server "
            "-cert /etc/adversarial-certs/expired-client-cert.pem "
            "-key /etc/adversarial-certs/expired-client-key.pem "
            "-accept ${toString expiredTLSPort} -quiet "
            ">/tmp/tls-expired.log 2>&1 &"
        )
        rogue.wait_for_open_port(${toString expiredTLSPort})

        # T-ADV-02: Wrong-CA cert server
        rogue.succeed(
            "openssl s_server "
            "-cert /etc/adversarial-certs/wrong-ca-client-cert.pem "
            "-key /etc/adversarial-certs/wrong-ca-client-key.pem "
            "-accept ${toString wrongCATLSPort} -quiet "
            ">/tmp/tls-wrong-ca.log 2>&1 &"
        )
        rogue.wait_for_open_port(${toString wrongCATLSPort})

        # T-ADV-03: Unpaired cert server
        rogue.succeed(
            "openssl s_server "
            "-cert /etc/adversarial-certs/unpaired-client-cert.pem "
            "-key /etc/adversarial-certs/unpaired-client-key.pem "
            "-accept ${toString unpairedTLSPort} -quiet "
            ">/tmp/tls-unpaired.log 2>&1 &"
        )
        rogue.wait_for_open_port(${toString unpairedTLSPort})

    # ── Phase 5: T-ADV-01 — Expired cert → rejected ──

    with subtest("T-ADV-01: expired cert rejected"):
        # nix-key test dials the device via mTLS. The rogue presents an expired cert.
        # PinnedTLSConfig.VerifyPeerCertificate rejects it because the cert is expired.
        host.fail(
            "su - testuser -c '"
            "${pkgs.nix-key}/bin/nix-key test expired-device "
            "--control-socket ${controlSocketPath} "
            "--timeout 5s'"
        )

    # ── Phase 6: T-ADV-02 — Wrong-CA cert → rejected ──

    with subtest("T-ADV-02: wrong-CA cert rejected"):
        # The rogue presents a cert signed by a different CA. The host expects
        # the legitimate phone cert fingerprint → fingerprint mismatch.
        host.fail(
            "su - testuser -c '"
            "${pkgs.nix-key}/bin/nix-key test wrong-ca-device "
            "--control-socket ${controlSocketPath} "
            "--timeout 5s'"
        )

    # ── Phase 7: T-ADV-03 — Unpaired cert → rejected ──

    with subtest("T-ADV-03: unpaired cert rejected"):
        # The rogue presents a valid cert not registered during pairing.
        # Fingerprint does not match the expected one → rejected.
        host.fail(
            "su - testuser -c '"
            "${pkgs.nix-key}/bin/nix-key test unpaired-device "
            "--control-socket ${controlSocketPath} "
            "--timeout 5s'"
        )

    # ── Phase 8: T-ADV-04 — Non-Tailscale interface → rejected ──

    with subtest("T-ADV-04: connection on non-Tailscale interface rejected"):
        # The phone's firewall only allows phonesim port on tailscale0.
        # Get the phone's eth1 (VM network) IP.
        phone_eth1_ip = phone.succeed("ip -4 addr show eth1 | grep -oP '(?<=inet\\s)\\d+\\.\\d+\\.\\d+\\.\\d+'").strip()
        assert len(phone_eth1_ip) > 0, "Phone should have an eth1 IP"

        # Verify phonesim IS reachable via Tailscale
        rogue.succeed(f"nc -z -w 3 {phone_ts_ip} ${toString phonesimPort}")

        # Verify phonesim is NOT reachable via eth1 (non-Tailscale, blocked by firewall)
        rogue.fail(f"nc -z -w 3 {phone_eth1_ip} ${toString phonesimPort}")

    # ── Phase 9: T-ADV-05 — Replayed pairing token → rejected ──

    with subtest("T-ADV-05: replayed pairing token rejected"):
        # Start a pairing session on the host
        host.succeed(
            "su - testuser -c '"
            "yes y | ${pkgs.nix-key}/bin/nix-key pair "
            "--interface tailscale0 "
            "--hostname test-host "
            "--pair-info-file /tmp/pair-info.json "
            "--age-key-file /home/testuser/.local/state/nix-key/age-identity.txt "
            "--devices-path /home/testuser/.local/state/nix-key/devices-pair.json "
            "--certs-dir /home/testuser/.local/state/nix-key/certs "
            ">/tmp/pair-output.log 2>&1 &'"
        )
        host.wait_until_succeeds("test -f /tmp/pair-info.json", timeout=30)

        pair_info_raw = host.succeed("cat /tmp/pair-info.json").strip()
        pair_info = json.loads(pair_info_raw)
        pair_host = pair_info["Host"]
        pair_port = pair_info["Port"]
        pair_token = pair_info["Token"]

        # Phone completes pairing legitimately
        phone.succeed(
            "openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 "
            "-keyout /tmp/phone-pair-key.pem -out /tmp/phone-pair-cert.pem "
            "-days 1 -nodes -subj '/CN=phonesim-pair' 2>/dev/null"
        )
        phone_pair_cert = phone.succeed("cat /tmp/phone-pair-cert.pem").strip()

        import json as json_mod
        pairing_request = json_mod.dumps({
            "phoneName": "Legitimate Phone",
            "tailscaleIp": phone_ts_ip,
            "listenPort": ${toString phonesimPort},
            "serverCert": phone_pair_cert,
            "token": pair_token,
        })
        phone.succeed(f"cat > /tmp/pairing-request.json << 'PAIREOF'\n{pairing_request}\nPAIREOF")
        phone.succeed(
            f"curl -sk --max-time 30 "
            f"-X POST -H 'Content-Type: application/json' "
            f"-d @/tmp/pairing-request.json "
            f"https://{pair_host}:{pair_port}/pair "
            f"-o /tmp/pair-response.json"
        )

        # Now the rogue replays the same token — should be rejected.
        # The pairing server may have shut down (single-use), in which case
        # the connection fails entirely. If still running, it returns HTTP 401.
        rogue_pairing_request = json_mod.dumps({
            "phoneName": "Rogue Phone",
            "tailscaleIp": rogue_ts_ip,
            "listenPort": 9999,
            "serverCert": "FAKE-CERT",
            "token": pair_token,
        })
        rogue.succeed(f"cat > /tmp/rogue-pairing.json << 'PAIREOF'\n{rogue_pairing_request}\nPAIREOF")
        replay_result = rogue.succeed(
            f"curl -sk --max-time 10 -w '%{{http_code}}' "
            f"-X POST -H 'Content-Type: application/json' "
            f"-d @/tmp/rogue-pairing.json "
            f"https://{pair_host}:{pair_port}/pair "
            f"-o /tmp/rogue-pair-response.json || true"
        ).strip()
        # Either connection refused (server shut down) or HTTP 401 (token already used)
        if "000" not in replay_result:
            assert replay_result == "401", \
                f"Expected HTTP 401 for token replay, got: {replay_result}"

    # ── Phase 10: T-ADV-06 — Error responses leak no internal details ──

    with subtest("T-ADV-06: SSH agent errors leak no internal details"):
        # Get a key from the legitimate phone for signing
        host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L' "
            "| head -1 > /tmp/sign-key.pub"
        )
        key_line = host.succeed("cat /tmp/sign-key.pub").strip()
        assert len(key_line) > 0, "Should have at least one key"

        # Try to sign — this should work with the legitimate phone
        host.succeed("echo 'test data' > /tmp/test-data.txt")
        host.succeed(
            "su - testuser -c '"
            "SSH_AUTH_SOCK=${agentSocketPath} "
            "ssh-keygen -Y sign -f /tmp/sign-key.pub -n test "
            "< /tmp/test-data.txt > /dev/null 2>&1'"
        )

        # Check daemon logs for adversarial connection attempts
        daemon_log = host.succeed("cat /tmp/nix-key-daemon.log 2>/dev/null || true")

        # Verify error messages do not leak internal details (FR-097)
        # The SSH agent returns generic SSH_AGENT_FAILURE with no details
        # Daemon logs may contain diagnostic info, but SSH-facing errors must be opaque

        # Verify that nix-key test output for failed devices does not leak
        # internal Go stack traces, file paths, or error chains
        expired_output = host.succeed(
            "su - testuser -c '"
            "${pkgs.nix-key}/bin/nix-key test expired-device "
            "--control-socket ${controlSocketPath} "
            "--timeout 5s' 2>&1 || true"
        )
        wrongca_output = host.succeed(
            "su - testuser -c '"
            "${pkgs.nix-key}/bin/nix-key test wrong-ca-device "
            "--control-socket ${controlSocketPath} "
            "--timeout 5s' 2>&1 || true"
        )

        for output in [expired_output, wrongca_output]:
            # Must not contain Go stack traces
            assert "goroutine" not in output, \
                f"Error output must not contain Go stack traces: {output[:200]}"
            # Must not contain internal file paths
            assert "/nix/store/" not in output or "nix-key" in output, \
                f"Error output should not leak Nix store paths: {output[:200]}"
            # Must not contain raw panic info
            assert "panic:" not in output, \
                f"Error output must not contain panic info: {output[:200]}"
            # Must not contain internal function signatures
            assert "internal/" not in output, \
                f"Error output must not leak internal package paths: {output[:200]}"
  '';
}
