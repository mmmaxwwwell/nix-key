# NixOS VM test for nix-key Jaeger tracing integration.
# Validates: FR-068 — services.nix-key.tracing.jaeger.enable starts Jaeger
# and configures otelEndpoint to localhost:4317.
{ pkgs, nixKeyModule }:
{
  name = "nix-key-jaeger";

  nodes.machine =
    { config, lib, ... }:
    {
      imports = [ nixKeyModule ];

      services.nix-key = {
        enable = true;
        package = pkgs.nix-key;
        tracing.jaeger.enable = true;
        tracing.jaeger.package = pkgs.jaeger;
      };

      # Regular user for the user service
      users.users.testuser = {
        isNormalUser = true;
        uid = 1000;
        group = "testuser";
      };
      users.groups.testuser = { };

      # Enable lingering so user service manager starts at boot
      systemd.tmpfiles.rules = [
        "f /var/lib/systemd/linger/testuser - - - -"
      ];

      # curl is needed for trace submission test, jq for polling Jaeger query API
      environment.systemPackages = [ pkgs.curl pkgs.jq ];
    };

  testScript = ''
    import json

    machine.start()
    machine.wait_for_unit("multi-user.target")

    # ── Jaeger service starts and is healthy ──

    with subtest("Jaeger all-in-one service starts"):
        machine.wait_for_unit("jaeger-all-in-one.service")

    with subtest("Jaeger listens on OTLP gRPC port 4317"):
        machine.wait_for_open_port(4317)

    with subtest("Jaeger listens on query HTTP port 16686"):
        machine.wait_for_open_port(16686)

    # ── config.json has otelEndpoint set automatically ──

    with subtest("config.json otelEndpoint is localhost:4317"):
        config_raw = machine.succeed(
            "find /nix/store -maxdepth 1 -name '*nix-key-config.json' | head -1 | xargs cat"
        ).strip()
        config = json.loads(config_raw)
        assert config["otelEndpoint"] == "localhost:4317", \
            f"Expected otelEndpoint 'localhost:4317', got {config['otelEndpoint']}"

    with subtest("config.json jaegerEnable is true"):
        assert config["jaegerEnable"] is True, \
            f"Expected jaegerEnable true, got {config['jaegerEnable']}"

    # ── nix-key-agent user service has OTEL env var ──

    with subtest("nix-key-agent service has NIXKEY_OTEL_ENDPOINT"):
        machine.wait_until_succeeds(
            "systemctl --user -M testuser@ is-system-running 2>&1 | grep -qE 'running|degraded'",
            timeout=30,
        )
        unit = machine.succeed(
            "cat /etc/systemd/user/nix-key-agent.service"
        )
        assert "NIXKEY_OTEL_ENDPOINT" in unit, \
            "nix-key-agent service should have NIXKEY_OTEL_ENDPOINT env var"
        assert "localhost:4317" in unit, \
            "NIXKEY_OTEL_ENDPOINT should be localhost:4317"

    # ── Jaeger accepts traces via OTLP ──

    with subtest("Jaeger accepts OTLP traces via HTTP"):
        # Jaeger all-in-one listens on 4318 for OTLP HTTP.
        # Send a minimal OTLP trace export request.
        trace_payload = json.dumps({
            "resourceSpans": [{
                "resource": {
                    "attributes": [{
                        "key": "service.name",
                        "value": {"stringValue": "nix-key-test"}
                    }]
                },
                "scopeSpans": [{
                    "scope": {"name": "test"},
                    "spans": [{
                        "traceId": "0af7651916cd43dd8448eb211c80319c",
                        "spanId": "b7ad6b7169203331",
                        "name": "test-span",
                        "kind": 1,
                        "startTimeUnixNano": "1000000000",
                        "endTimeUnixNano": "2000000000",
                        "status": {}
                    }]
                }]
            }]
        })
        # Write payload to a file to avoid shell quoting issues
        machine.succeed(
            f"printf '%s' '{trace_payload}' > /tmp/trace.json"
        )
        # Submit trace via OTLP HTTP endpoint
        result = machine.succeed(
            "curl -s -w '\\n%{http_code}' "
            "-X POST http://localhost:4318/v1/traces "
            "-H 'Content-Type: application/json' "
            "-d @/tmp/trace.json"
        ).strip()
        lines = result.split("\n")
        http_code = lines[-1]
        assert http_code == "200", \
            f"Expected HTTP 200 from OTLP endpoint, got {http_code}: {result}"

    with subtest("Jaeger query API returns the submitted trace"):
        # Poll until Jaeger indexes the trace (up to 30s) — fixed sleep was
        # insufficient in resource-constrained CI VMs
        machine.wait_until_succeeds(
            "curl -s 'http://localhost:16686/api/traces?service=nix-key-test&limit=1' | jq -e '.data | length > 0'",
            timeout=120,
        )
        query_result = machine.succeed(
            "curl -s 'http://localhost:16686/api/traces?service=nix-key-test&limit=1'"
        ).strip()
        query_data = json.loads(query_result)
        trace = query_data["data"][0]
        spans = trace.get("spans", [])
        assert any(s["operationName"] == "test-span" for s in spans), \
            f"Expected 'test-span' in trace spans, got: {[s['operationName'] for s in spans]}"
  '';
}
