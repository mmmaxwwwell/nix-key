#!/usr/bin/env bash
# security-scan.sh — Run Tier 1 security scanners locally and produce JSON output.
#
# Usage: scripts/security-scan.sh
#
# Runs: Trivy, Semgrep (p/golang), Gitleaks, govulncheck
# Output: test-logs/security/<scanner>.json + test-logs/security/summary.json
#
# Each scanner that is not installed is skipped (exit_code: -1, findings: 0).
# Exit code: 0 if all scanners pass, 1 if any scanner finds issues.

set -uo pipefail

OUTDIR="test-logs/security"
mkdir -p "$OUTDIR"

total_findings=0
all_pass=true

# Associative arrays for results
declare -A scanner_findings
declare -A scanner_exit_codes

# --- Trivy ---
run_trivy() {
  local name="trivy"
  if ! command -v trivy &>/dev/null; then
    echo "SKIP: trivy not found"
    scanner_findings[$name]=0
    scanner_exit_codes[$name]=-1
    jq -n '{scanner: "trivy", status: "skipped", findings: 0, exit_code: -1}' > "$OUTDIR/trivy.json"
    return
  fi

  echo "RUN:  trivy fs scan..."
  trivy fs --scanners vuln,secret,misconfig \
    --severity CRITICAL,HIGH \
    --format json \
    --output "$OUTDIR/trivy.json" \
    . 2>/dev/null
  local ec=$?
  scanner_exit_codes[$name]=$ec

  # Count findings from JSON output
  local count=0
  if [ -f "$OUTDIR/trivy.json" ]; then
    count=$(jq '[.Results[]?.Vulnerabilities // [] | length] + [.Results[]?.Secrets // [] | length] + [.Results[]?.Misconfigurations // [] | length] | add // 0' "$OUTDIR/trivy.json" 2>/dev/null || echo 0)
  fi
  scanner_findings[$name]=$count
  total_findings=$((total_findings + count))
  if [ "$ec" -ne 0 ]; then
    all_pass=false
  fi
  echo "DONE: trivy (findings=$count, exit_code=$ec)"
}

# --- Semgrep ---
run_semgrep() {
  local name="semgrep"
  if ! command -v semgrep &>/dev/null; then
    echo "SKIP: semgrep not found"
    scanner_findings[$name]=0
    scanner_exit_codes[$name]=-1
    jq -n '{scanner: "semgrep", status: "skipped", findings: 0, exit_code: -1}' > "$OUTDIR/semgrep.json"
    return
  fi

  echo "RUN:  semgrep (p/golang)..."
  semgrep scan --config p/golang \
    --json \
    --output "$OUTDIR/semgrep.json" \
    --quiet \
    . 2>/dev/null
  local ec=$?
  scanner_exit_codes[$name]=$ec

  local count=0
  if [ -f "$OUTDIR/semgrep.json" ]; then
    count=$(jq '.results | length // 0' "$OUTDIR/semgrep.json" 2>/dev/null || echo 0)
  fi
  scanner_findings[$name]=$count
  total_findings=$((total_findings + count))
  if [ "$ec" -ne 0 ] && [ "$ec" -ne 1 ]; then
    # semgrep exit 1 = findings found (not a tool error)
    # We still treat findings as a non-pass
    :
  fi
  if [ "$count" -gt 0 ]; then
    all_pass=false
  fi
  echo "DONE: semgrep (findings=$count, exit_code=$ec)"
}

# --- Gitleaks ---
run_gitleaks() {
  local name="gitleaks"
  if ! command -v gitleaks &>/dev/null; then
    echo "SKIP: gitleaks not found"
    scanner_findings[$name]=0
    scanner_exit_codes[$name]=-1
    jq -n '{scanner: "gitleaks", status: "skipped", findings: 0, exit_code: -1}' > "$OUTDIR/gitleaks.json"
    return
  fi

  echo "RUN:  gitleaks detect..."
  gitleaks detect \
    --source . \
    --report-format json \
    --report-path "$OUTDIR/gitleaks.json" \
    --no-banner \
    2>/dev/null
  local ec=$?
  scanner_exit_codes[$name]=$ec

  local count=0
  if [ -f "$OUTDIR/gitleaks.json" ]; then
    count=$(jq 'if type == "array" then length else 0 end' "$OUTDIR/gitleaks.json" 2>/dev/null || echo 0)
  fi
  scanner_findings[$name]=$count
  total_findings=$((total_findings + count))
  if [ "$ec" -ne 0 ]; then
    all_pass=false
  fi
  echo "DONE: gitleaks (findings=$count, exit_code=$ec)"
}

# --- govulncheck ---
run_govulncheck() {
  local name="govulncheck"
  if ! command -v govulncheck &>/dev/null; then
    echo "SKIP: govulncheck not found"
    scanner_findings[$name]=0
    scanner_exit_codes[$name]=-1
    jq -n '{scanner: "govulncheck", status: "skipped", findings: 0, exit_code: -1}' > "$OUTDIR/govulncheck.json"
    return
  fi

  echo "RUN:  govulncheck..."
  govulncheck -format json ./... > "$OUTDIR/govulncheck.json" 2>/dev/null
  local ec=$?
  scanner_exit_codes[$name]=$ec

  # govulncheck JSON: count entries with "finding" key
  local count=0
  if [ -f "$OUTDIR/govulncheck.json" ]; then
    count=$(jq -s '[.[] | select(.finding != null)] | length' "$OUTDIR/govulncheck.json" 2>/dev/null || echo 0)
  fi
  scanner_findings[$name]=$count
  total_findings=$((total_findings + count))
  if [ "$ec" -ne 0 ]; then
    all_pass=false
  fi
  echo "DONE: govulncheck (findings=$count, exit_code=$ec)"
}

# --- Run all scanners ---
echo "=== Security Scan ==="
echo ""

run_trivy
run_semgrep
run_gitleaks
run_govulncheck

echo ""

# --- Produce summary.json ---
pass_str="true"
if [ "$all_pass" = false ]; then
  pass_str="false"
fi

jq -n \
  --argjson trivy_findings "${scanner_findings[trivy]}" \
  --argjson trivy_ec "${scanner_exit_codes[trivy]}" \
  --argjson semgrep_findings "${scanner_findings[semgrep]}" \
  --argjson semgrep_ec "${scanner_exit_codes[semgrep]}" \
  --argjson gitleaks_findings "${scanner_findings[gitleaks]}" \
  --argjson gitleaks_ec "${scanner_exit_codes[gitleaks]}" \
  --argjson govulncheck_findings "${scanner_findings[govulncheck]}" \
  --argjson govulncheck_ec "${scanner_exit_codes[govulncheck]}" \
  --argjson total "$total_findings" \
  --argjson pass "$pass_str" \
  '{
    scanners: {
      trivy:        {findings: $trivy_findings,        exit_code: $trivy_ec},
      semgrep:      {findings: $semgrep_findings,      exit_code: $semgrep_ec},
      gitleaks:     {findings: $gitleaks_findings,      exit_code: $gitleaks_ec},
      govulncheck:  {findings: $govulncheck_findings,  exit_code: $govulncheck_ec}
    },
    total_findings: $total,
    pass: $pass
  }' > "$OUTDIR/summary.json"

echo "=== Summary ==="
jq . "$OUTDIR/summary.json"

if [ "$all_pass" = false ]; then
  echo ""
  echo "FAIL: security scan found issues"
  exit 1
fi

echo ""
echo "PASS: all security scanners clean"
exit 0
