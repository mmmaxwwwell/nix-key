#!/usr/bin/env bash
# ci-summary.sh — Collect test-logs/summary.json from CI job artifacts and
# produce a unified ci-summary.json for fix-validate agents.
#
# Usage (in GitHub Actions):
#   scripts/ci-summary.sh <artifacts-dir> <output-file>
#
# Environment variables (set by the workflow):
#   JOB_LINT          - "success", "failure", "cancelled", or "skipped"
#   JOB_TEST_HOST     - same
#   JOB_TEST_ANDROID  - same
#   JOB_SECURITY      - same
#
# The <artifacts-dir> should contain subdirectories named after artifact names
# (e.g., "test-host-logs/", "test-android-logs/") downloaded by
# actions/download-artifact.

set -euo pipefail

ARTIFACTS_DIR="${1:-.}"
OUTPUT_FILE="${2:-ci-summary.json}"

# build_job_entry constructs a JSON object for one job.
# Args: name, result, summary_json_path (optional)
build_job_entry() {
  local name="$1"
  local result="$2"
  local summary_path="${3:-}"

  if [ -n "$summary_path" ] && [ -f "$summary_path" ]; then
    # Extract fields from structured summary.json
    local pass fail skip duration failures
    pass=$(jq -r '.pass // 0' "$summary_path")
    fail=$(jq -r '.fail // 0' "$summary_path")
    skip=$(jq -r '.skip // 0' "$summary_path")
    duration=$(jq -r '.duration // 0' "$summary_path")
    failures=$(jq -c '[.failures[]? | {package, test, elapsed}]' "$summary_path")

    jq -n \
      --arg name "$name" \
      --argjson pass "$pass" \
      --argjson fail "$fail" \
      --argjson skip "$skip" \
      --argjson duration "$duration" \
      --argjson failures "$failures" \
      '{name: $name, pass: $pass, fail: $fail, skip: $skip, duration: $duration, failures: $failures}'
  else
    # No structured output — derive from job result
    local pass=0 fail=0
    if [ "$result" = "success" ]; then
      pass=1
    else
      fail=1
    fi

    jq -n \
      --arg name "$name" \
      --argjson pass "$pass" \
      --argjson fail "$fail" \
      '{name: $name, pass: $pass, fail: $fail, skip: 0, duration: 0, failures: []}'
  fi
}

# find_summary searches for summary.json inside an artifact directory.
# Artifacts are downloaded into <artifacts-dir>/<artifact-name>/...
find_summary() {
  local artifact_name="$1"
  local search_dir="${ARTIFACTS_DIR}/${artifact_name}"

  if [ ! -d "$search_dir" ]; then
    echo ""
    return
  fi

  # summary.json lives at test-logs/<type>/<timestamp>/summary.json
  # or test-logs/ci/latest/summary.json
  local found
  found=$(find "$search_dir" -name "summary.json" -type f 2>/dev/null | head -1)
  echo "${found:-}"
}

# --- Build job entries ---

jobs_json="[]"

# Lint job
lint_result="${JOB_LINT:-skipped}"
lint_summary=$(find_summary "lint-test-logs")
lint_entry=$(build_job_entry "lint" "$lint_result" "$lint_summary")
jobs_json=$(echo "$jobs_json" | jq --argjson entry "$lint_entry" '. + [$entry]')

# Test Host job
test_host_result="${JOB_TEST_HOST:-skipped}"
test_host_summary=$(find_summary "test-host-logs")
test_host_entry=$(build_job_entry "test-host" "$test_host_result" "$test_host_summary")
jobs_json=$(echo "$jobs_json" | jq --argjson entry "$test_host_entry" '. + [$entry]')

# Test Android job
test_android_result="${JOB_TEST_ANDROID:-skipped}"
test_android_summary=$(find_summary "test-android-logs")
test_android_entry=$(build_job_entry "test-android" "$test_android_result" "$test_android_summary")
jobs_json=$(echo "$jobs_json" | jq --argjson entry "$test_android_entry" '. + [$entry]')

# Security job
security_result="${JOB_SECURITY:-skipped}"
security_summary=$(find_summary "security-logs")
security_entry=$(build_job_entry "security" "$security_result" "$security_summary")

# Enrich security entry with per-scanner finding counts if available
security_scan_summary=""
if [ -d "${ARTIFACTS_DIR}/security-logs" ]; then
  security_scan_summary=$(find "${ARTIFACTS_DIR}/security-logs" -path "*/security/summary.json" -type f 2>/dev/null | head -1)
fi
if [ -n "$security_scan_summary" ] && [ -f "$security_scan_summary" ]; then
  scanners_json=$(jq -c '.scanners' "$security_scan_summary")
  total_findings=$(jq -r '.total_findings // 0' "$security_scan_summary")
  scan_pass=$(jq -r '.pass // true' "$security_scan_summary")
  security_entry=$(echo "$security_entry" | jq \
    --argjson scanners "$scanners_json" \
    --argjson total_findings "$total_findings" \
    --argjson scan_pass "$scan_pass" \
    '. + {scanners: $scanners, total_findings: $total_findings, scan_pass: $scan_pass}')
fi

jobs_json=$(echo "$jobs_json" | jq --argjson entry "$security_entry" '. + [$entry]')

# --- Determine overall status ---

overall="pass"
for result_var in "$lint_result" "$test_host_result" "$test_android_result" "$security_result"; do
  if [ "$result_var" != "success" ] && [ "$result_var" != "skipped" ]; then
    overall="fail"
    break
  fi
done

# --- Produce ci-summary.json ---

jq -n \
  --argjson jobs "$jobs_json" \
  --arg overall "$overall" \
  '{jobs: $jobs, overall: $overall, artifactUrls: {}}' \
  > "$OUTPUT_FILE"

echo "ci-summary: wrote $OUTPUT_FILE"
echo "ci-summary: overall=$overall"
