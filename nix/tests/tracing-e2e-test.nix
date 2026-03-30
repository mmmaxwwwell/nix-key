# NixOS VM test for distributed OTEL tracing across host daemon + phonesim.
# Validates: T-E2E-03, Story 7, SC-008.
#
# Test topology:
#   host node: headscale + tailscaled + nix-key daemon (OTEL) + Jaeger all-in-one
#   phone node: tailscaled + phonesim (OTEL → host Jaeger)
#
# Scenario:
#   1. Set up Tailnet (headscale) and join both nodes
#   2. Start Jaeger on host
#   3. Start phonesim with -otel-endpoint pointing to host's Jaeger (via Tailscale IP)
#   4. Start nix-key daemon with OTEL endpoint pointing to localhost Jaeger
#   5. Perform SSH sign request
#   6. Query Jaeger API for traces
#   7. Verify: trace exists, host spans present, phone spans present,
#      phone spans are children of host spans (same traceId, parent-child via traceparent)
{ pkgs, nixKeyModule }:
let
  headscaleDomain = "headscale.test";
  headscalePort = 8080;

  phonesimPkg = pkgs.phonesim;
  phonesimPort = 50051;

  agentSocketPath = "/tmp/nix-key-test/agent.sock";
in
{
  name = "nix-key-tracing-e2e";

  nodes.host =
    { config, lib, ... }:
    {
      imports = [ nixKeyModule ];

      services.nix-key = {
        enable = true;
        package = pkgs.nix-key;
        tailscaleInterface = "tailscale0";
        logLevel = "debug";
        signTimeout = 30;
        connectionTimeout = 10;
        socketPath = agentSocketPath;
        secrets.ageKeyFile = "/tmp/test-age-identity.txt";
        tracing.jaeger.enable = true;
        tracing.jaeger.package = pkgs.jaeger;
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

      services.tailscale.enable = true;

      networking.extraHosts = ''
        127.0.0.1 ${headscaleDomain}
      '';

      networking.firewall.enable = false;

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
        pkgs.curl
      ];
    };

  nodes.phone =
    { config, lib, ... }:
    {
      services.tailscale.enable = true;

      networking.extraHosts = ''
        192.168.1.1 ${headscaleDomain}
      '';

      networking.firewall.enable = false;

      environment.systemPackages = [
        phonesimPkg
        pkgs.jq
        pkgs.curl
      ];
    };

  testScript = ''
    import json
    import time

    start_all()

    # ── Phase 1: Infrastructure setup ──

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

    with subtest("Jaeger starts"):
        host.wait_for_unit("jaeger-all-in-one.service")
        host.wait_for_open_port(4317)   # OTLP gRPC
        host.wait_for_open_port(16686)  # Jaeger query UI

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

    # ── Phase 3: Start phonesim with OTEL ──
    # phonesim exports traces to Jaeger on the host node via the host's Tailscale IP.

    with subtest("start phonesim with OTEL"):
        phone.succeed(
            f"phonesim -plain-listen 0.0.0.0:${toString phonesimPort} "
            f"-otel-endpoint {host_ts_ip}:4317 "
            f">/tmp/phonesim.log 2>&1 &"
        )
        phone.wait_for_open_port(${toString phonesimPort})

    # ── Phase 4: Create pre-paired state on host ──

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

        # Symlink the Nix-generated config.json
        config_store_path = host.succeed(
            "find /nix/store -maxdepth 1 -name '*nix-key-config.json' | head -1"
        ).strip()
        host.succeed(
            f"ln -sf {config_store_path} /home/testuser/.config/nix-key/config.json "
            f"&& chown -h testuser:testuser /home/testuser/.config/nix-key/config.json"
        )

    # ── Phase 5: Start nix-key daemon with OTEL ──

    with subtest("start nix-key daemon with OTEL"):
        host.succeed(
            "su - testuser -c '"
            "NIXKEY_OTEL_ENDPOINT=localhost:4317 "
            "${pkgs.nix-key}/bin/nix-key daemon "
            "--config /home/testuser/.config/nix-key/config.json "
            ">/tmp/nix-key-daemon.log 2>&1 &'"
        )
        host.wait_until_succeeds(
            "test -S ${agentSocketPath}",
            timeout=30,
        )

    # ── Phase 6: Perform sign request ──

    with subtest("list keys from phonesim"):
        keys_output = host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L'"
        ).strip()
        assert "ssh-ed25519" in keys_output or "ecdsa-sha2-nistp256" in keys_output, \
            f"Expected SSH key types in output, got: {keys_output}"
        host.succeed(
            "su - testuser -c 'SSH_AUTH_SOCK=${agentSocketPath} ssh-add -L' "
            "| head -1 > /tmp/sign-key.pub"
        )

    with subtest("perform SSH sign operation"):
        host.succeed("echo 'test data for tracing' > /tmp/test-data.txt")
        host.succeed(
            "su - testuser -c '"
            "SSH_AUTH_SOCK=${agentSocketPath} "
            "ssh-keygen -Y sign -f /tmp/sign-key.pub -n test "
            "< /tmp/test-data.txt > /tmp/test-signature'"
        )
        sig_output = host.succeed("cat /tmp/test-signature").strip()
        assert "BEGIN SSH SIGNATURE" in sig_output, \
            f"Expected SSH signature block, got: {sig_output[:200]}"

    # ── Phase 7: Query Jaeger and verify distributed traces ──

    with subtest("wait for traces to be indexed"):
        # OTLP batchers flush periodically; give them time to export.
        # Also give Jaeger time to index the traces.
        time.sleep(5)

    with subtest("query Jaeger for host traces"):
        # Query for traces from the host service ("nix-key")
        host_traces_raw = host.wait_until_succeeds(
            "curl -sf 'http://localhost:16686/api/traces?service=nix-key&limit=10' "
            "| jq '.data | length' ",
            timeout=30,
        ).strip()
        host_trace_count = int(host_traces_raw)
        assert host_trace_count > 0, \
            f"Expected at least one trace from nix-key service, got {host_trace_count}"

    with subtest("query Jaeger for phone traces"):
        # Query for traces from the phone service ("nix-key-phone")
        phone_traces_raw = host.wait_until_succeeds(
            "curl -sf 'http://localhost:16686/api/traces?service=nix-key-phone&limit=10' "
            "| jq '.data | length' ",
            timeout=30,
        ).strip()
        phone_trace_count = int(phone_traces_raw)
        assert phone_trace_count > 0, \
            f"Expected at least one trace from nix-key-phone service, got {phone_trace_count}"

    with subtest("verify distributed trace with host and phone spans"):
        # Get all traces from the host service — they should include phone spans
        # because W3C traceparent propagation links them into the same trace.
        traces_raw = host.succeed(
            "curl -sf 'http://localhost:16686/api/traces?service=nix-key&limit=10'"
        ).strip()
        traces = json.loads(traces_raw)

        # Find a trace that contains spans from both services
        found_distributed_trace = False
        for trace in traces["data"]:
            processes = trace.get("processes", {})
            service_names = set()
            for proc in processes.values():
                sn = proc.get("serviceName", "")
                if sn:
                    service_names.add(sn)

            if "nix-key" in service_names and "nix-key-phone" in service_names:
                found_distributed_trace = True

                # Verify parent-child relationship:
                # Phone spans should have a reference to a host span as parent.
                spans = trace.get("spans", [])

                # Build map of spanID -> span for lookup
                span_map = {s["spanID"]: s for s in spans}

                # Find phone process IDs
                phone_process_ids = set()
                host_process_ids = set()
                for pid, proc in processes.items():
                    if proc.get("serviceName") == "nix-key-phone":
                        phone_process_ids.add(pid)
                    elif proc.get("serviceName") == "nix-key":
                        host_process_ids.add(pid)

                # Phone spans
                phone_spans = [s for s in spans if s.get("processID") in phone_process_ids]
                host_spans = [s for s in spans if s.get("processID") in host_process_ids]

                assert len(host_spans) > 0, "Expected host spans in distributed trace"
                assert len(phone_spans) > 0, "Expected phone spans in distributed trace"

                # Verify at least one phone span has a CHILD_OF reference
                # to a host span (traceparent propagation)
                phone_has_host_parent = False
                for ps in phone_spans:
                    for ref in ps.get("references", []):
                        if ref.get("refType") == "CHILD_OF" and ref.get("spanID") in span_map:
                            parent = span_map[ref["spanID"]]
                            if parent.get("processID") in host_process_ids:
                                phone_has_host_parent = True
                                break
                    if phone_has_host_parent:
                        break

                assert phone_has_host_parent, \
                    "Expected phone spans to be children of host spans (traceparent propagation)"
                break

        assert found_distributed_trace, \
            "Expected a distributed trace containing both nix-key and nix-key-phone services"

    with subtest("verify expected span names present"):
        # Re-use the distributed trace found above.
        # Check for expected host span names from T049.
        traces_raw = host.succeed(
            "curl -sf 'http://localhost:16686/api/traces?service=nix-key&limit=10'"
        ).strip()
        traces = json.loads(traces_raw)

        all_span_names = set()
        for trace in traces["data"]:
            for span in trace.get("spans", []):
                all_span_names.add(span.get("operationName", ""))

        # Host spans (from T049): ssh-sign-request, device-lookup, mtls-connect, return-signature
        # Phone spans (from T050): handle-sign-request, keystore-sign
        # The otelgrpc interceptor also creates automatic RPC spans.
        # We verify the most distinctive ones are present.
        for expected in ["handle-sign-request", "keystore-sign"]:
            assert expected in all_span_names, \
                f"Expected span '{expected}' in trace, got: {sorted(all_span_names)}"
  '';
}
