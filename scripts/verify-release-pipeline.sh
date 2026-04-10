#!/usr/bin/env bash
# scripts/verify-release-pipeline.sh
#
# Verifies the release pipeline configuration end-to-end.
# Two modes:
#   --config-only   Validate config files only (no GitHub API calls)
#   --live          Validate config + check live GitHub state (requires gh auth)
#
# Usage:
#   ./scripts/verify-release-pipeline.sh [--config-only|--live] [OWNER/REPO]

set -euo pipefail

MODE="${1:---config-only}"
REPO="${2:-}"
PASS=0
FAIL=0
WARN=0

pass() { PASS=$((PASS + 1)); echo "  [PASS] $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  [FAIL] $1"; }
warn() { WARN=$((WARN + 1)); echo "  [WARN] $1"; }
section() { echo ""; echo "=== $1 ==="; }

# ---------- Config validation ----------

section "1. Workflow files exist"
for f in .github/workflows/ci.yml .github/workflows/e2e.yml .github/workflows/release.yml; do
  if [ -f "$f" ]; then
    pass "$f exists"
  else
    fail "$f missing"
  fi
done

section "2. release-please configuration"
if [ -f release-please-config.json ]; then
  pass "release-please-config.json exists"
  RELEASE_TYPE=$(jq -r '.packages["."]["release-type"]' release-please-config.json 2>/dev/null || echo "")
  if [ "$RELEASE_TYPE" = "go" ]; then
    pass "release-type is 'go'"
  else
    fail "release-type is '$RELEASE_TYPE' (expected 'go')"
  fi
else
  fail "release-please-config.json missing"
fi

if [ -f .release-please-manifest.json ]; then
  pass ".release-please-manifest.json exists"
  VERSION=$(jq -r '."."' .release-please-manifest.json 2>/dev/null || echo "")
  if [ -n "$VERSION" ]; then
    pass "manifest tracks version: $VERSION"
  else
    fail "manifest missing root package version"
  fi
else
  fail ".release-please-manifest.json missing"
fi

section "3. CI workflow triggers"
# CI must trigger on PRs to develop AND main, and on push to main
if grep -q 'pull_request:' .github/workflows/ci.yml && \
   grep -q 'branches: \[develop, main\]' .github/workflows/ci.yml; then
  pass "CI triggers on PRs to develop and main"
else
  fail "CI missing PR triggers for develop/main"
fi

if grep -q 'workflow_call:' .github/workflows/ci.yml; then
  pass "CI supports workflow_call (reusable)"
else
  fail "CI missing workflow_call trigger"
fi

if grep -q 'push:' .github/workflows/ci.yml && \
   grep -q 'branches: \[main\]' .github/workflows/ci.yml; then
  pass "CI triggers on push to main"
else
  fail "CI missing push-to-main trigger"
fi

section "4. E2E workflow triggers"
if grep -q 'push:' .github/workflows/e2e.yml && \
   grep -q 'branches: \[develop\]' .github/workflows/e2e.yml; then
  pass "E2E triggers on push to develop"
else
  fail "E2E missing push-to-develop trigger"
fi

if grep -q 'pull_request:' .github/workflows/e2e.yml && \
   grep -q 'branches: \[main\]' .github/workflows/e2e.yml; then
  pass "E2E triggers on PRs to main"
else
  fail "E2E missing PR-to-main trigger"
fi

if grep -q 'workflow_call:' .github/workflows/e2e.yml; then
  pass "E2E supports workflow_call (reusable)"
else
  fail "E2E missing workflow_call trigger"
fi

section "5. Release workflow configuration"
if grep -q 'push:' .github/workflows/release.yml && \
   grep -q 'branches: \[main\]' .github/workflows/release.yml; then
  pass "Release triggers on push to main"
else
  fail "Release missing push-to-main trigger"
fi

if grep -q 'cancel-in-progress: false' .github/workflows/release.yml; then
  pass "Release does not cancel in-progress runs"
else
  fail "Release should not cancel in-progress runs"
fi

if grep -q 'release-please-action@v4' .github/workflows/release.yml; then
  pass "Uses release-please-action v4"
else
  fail "Missing or wrong release-please-action version"
fi

section "6. Release workflow job DAG"
# release-please -> ci -> e2e -> build-go/build-apk -> upload-assets
if grep -q 'needs: release-please' .github/workflows/release.yml; then
  pass "CI depends on release-please"
else
  fail "CI missing dependency on release-please"
fi

if grep -q 'needs: \[release-please, ci\]' .github/workflows/release.yml; then
  pass "E2E depends on release-please + CI"
else
  fail "E2E missing dependency chain"
fi

if grep -q 'needs: \[release-please, ci, e2e\]' .github/workflows/release.yml; then
  pass "Build jobs depend on release-please + CI + E2E"
else
  fail "Build jobs missing full dependency chain"
fi

if grep -q 'needs: \[release-please, build-go, build-apk, sbom\]' .github/workflows/release.yml; then
  pass "Upload-assets depends on all build jobs + SBOM"
else
  fail "Upload-assets missing build job dependencies"
fi

section "7. Release artifacts"
# Check that all expected artifacts are produced
if grep -q 'nix-key-x86_64-linux' .github/workflows/release.yml; then
  pass "x86_64-linux binary artifact configured"
else
  fail "Missing x86_64-linux binary"
fi

if grep -q 'nix-key-aarch64-linux' .github/workflows/release.yml; then
  pass "aarch64-linux binary artifact configured"
else
  fail "Missing aarch64-linux binary"
fi

if grep -q 'assembleRelease' .github/workflows/release.yml; then
  pass "APK build (assembleRelease) configured"
else
  fail "Missing APK assembleRelease"
fi

if grep -q 'cyclonedx' .github/workflows/release.yml; then
  pass "CycloneDX SBOM generation configured"
else
  fail "Missing CycloneDX SBOM"
fi

if grep -q 'gh release upload' .github/workflows/release.yml; then
  pass "gh release upload configured for artifact attachment"
else
  fail "Missing gh release upload step"
fi

if grep -q -- '--clobber' .github/workflows/release.yml; then
  pass "Upload uses --clobber for idempotent retry"
else
  warn "Upload missing --clobber flag"
fi

section "8. CI jobs required by branch protection"
if [ -f scripts/setup-branch-protection.sh ]; then
  pass "Branch protection script exists"
  # Verify main requires all CI + security + E2E
  if grep -q '"Lint"' scripts/setup-branch-protection.sh && \
     grep -q '"Test Host"' scripts/setup-branch-protection.sh && \
     grep -q '"Test Android"' scripts/setup-branch-protection.sh && \
     grep -q '"Security Scan"' scripts/setup-branch-protection.sh && \
     grep -q '"CI Summary"' scripts/setup-branch-protection.sh && \
     grep -q '"Android Emulator E2E"' scripts/setup-branch-protection.sh; then
    pass "main branch requires all CI + security + E2E checks"
  else
    fail "main branch protection missing required checks"
  fi
else
  fail "scripts/setup-branch-protection.sh missing"
fi

section "9. Security: permissions and secrets"
if grep -q 'contents: write' .github/workflows/release.yml && \
   grep -q 'pull-requests: write' .github/workflows/release.yml; then
  pass "Release workflow has write permissions for contents + PRs"
else
  fail "Release workflow missing required permissions"
fi

if grep -q 'GITHUB_TOKEN' .github/workflows/release.yml; then
  pass "GH_TOKEN/GITHUB_TOKEN used for release upload"
else
  fail "Missing token for release upload"
fi

if grep -q 'secrets: inherit' .github/workflows/release.yml; then
  pass "Reusable workflow calls inherit secrets"
else
  fail "Reusable workflow calls missing 'secrets: inherit'"
fi

section "10. Pipeline flow validation"
echo "  Expected flow:"
echo "    1. Push to develop -> CI runs (lint, test-host, test-android, security)"
echo "    2. CI green -> E2E runs (on push to develop)"
echo "    3. PR from develop to main -> CI + E2E run on PR"
echo "    4. Merge PR to main -> Release workflow triggers"
echo "    5. release-please creates/updates release PR (or creates release)"
echo "    6. On release_created: CI -> E2E -> build-go + build-apk + sbom -> upload-assets"
echo "    7. GitHub Release created with binaries + APK + SBOM attached"
echo ""

# Verify the flow is connected
if grep -q 'release_created.*true' .github/workflows/release.yml; then
  pass "Build/upload gated on release_created == true"
else
  fail "Build jobs not gated on release_created"
fi

# ---------- Live validation (optional) ----------

if [ "$MODE" = "--live" ]; then
  section "11. Live GitHub state"

  if [ -z "$REPO" ]; then
    REPO=$(gh repo view --json nameWithOwner -q '.nameWithOwner' 2>/dev/null) || {
      fail "Could not detect repository. Pass OWNER/REPO as second argument."
      REPO=""
    }
  fi

  if [ -n "$REPO" ]; then
    echo "  Repository: $REPO"

    # Check if release-please PR exists
    RP_PRS=$(gh pr list --repo "$REPO" --label "autorelease: pending" --json number,title 2>/dev/null || echo "[]")
    RP_COUNT=$(echo "$RP_PRS" | jq 'length')
    if [ "$RP_COUNT" -gt 0 ]; then
      pass "Found $RP_COUNT release-please PR(s)"
      echo "$RP_PRS" | jq -r '.[] | "    PR #\(.number): \(.title)"'
    else
      warn "No pending release-please PRs found (expected after first push to main)"
    fi

    # Check latest workflow runs
    echo ""
    echo "  Recent workflow runs:"
    gh run list --repo "$REPO" --limit 5 --json name,status,conclusion,headBranch \
      --jq '.[] | "    \(.name) [\(.headBranch)] -> \(.status)/\(.conclusion)"' 2>/dev/null || \
      warn "Could not fetch workflow runs"

    # Check latest releases
    echo ""
    LATEST_RELEASE=$(gh release list --repo "$REPO" --limit 1 --json tagName,name,isDraft,isPrerelease 2>/dev/null || echo "[]")
    REL_COUNT=$(echo "$LATEST_RELEASE" | jq 'length')
    if [ "$REL_COUNT" -gt 0 ]; then
      TAG=$(echo "$LATEST_RELEASE" | jq -r '.[0].tagName')
      NAME=$(echo "$LATEST_RELEASE" | jq -r '.[0].name')
      pass "Latest release: $NAME ($TAG)"

      # Check release assets
      ASSETS=$(gh release view "$TAG" --repo "$REPO" --json assets --jq '.assets[].name' 2>/dev/null || echo "")
      if [ -n "$ASSETS" ]; then
        echo "  Release assets:"
        echo "$ASSETS" | while read -r asset; do
          echo "    - $asset"
        done

        # Verify expected assets
        echo "$ASSETS" | grep -q 'nix-key-x86_64-linux' && pass "x86_64-linux binary in release" || fail "x86_64-linux binary missing from release"
        echo "$ASSETS" | grep -q 'nix-key-aarch64-linux' && pass "aarch64-linux binary in release" || fail "aarch64-linux binary missing from release"
        echo "$ASSETS" | grep -q '\.apk$' && pass "APK in release" || fail "APK missing from release"
        echo "$ASSETS" | grep -q 'sbom' && pass "SBOM in release" || fail "SBOM missing from release"

        # Download and verify binary
        echo ""
        echo "  Downloading x86_64-linux binary for verification..."
        TMPDIR=$(mktemp -d)
        if gh release download "$TAG" --repo "$REPO" --pattern 'nix-key-x86_64-linux*' --dir "$TMPDIR" 2>/dev/null; then
          BIN=$(find "$TMPDIR" -name 'nix-key-x86_64-linux*' -type f | head -1)
          if [ -n "$BIN" ]; then
            chmod +x "$BIN"
            if "$BIN" --help >/dev/null 2>&1; then
              pass "nix-key --help runs successfully"
            else
              fail "nix-key --help failed (exit code: $?)"
            fi
          else
            fail "Could not find downloaded binary"
          fi
        else
          fail "Could not download binary from release"
        fi
        rm -rf "$TMPDIR"
      else
        warn "No release assets found (release may be draft or assets not yet uploaded)"
      fi
    else
      warn "No releases found (pipeline may not have completed a release cycle yet)"
    fi
  fi
fi

# ---------- Summary ----------

section "Summary"
TOTAL=$((PASS + FAIL + WARN))
echo "  Passed:   $PASS"
echo "  Failed:   $FAIL"
echo "  Warnings: $WARN"
echo "  Total:    $TOTAL"
echo ""

if [ "$FAIL" -gt 0 ]; then
  echo "RESULT: FAIL ($FAIL failures)"
  exit 1
elif [ "$WARN" -gt 0 ]; then
  echo "RESULT: PASS with warnings"
  exit 0
else
  echo "RESULT: PASS"
  exit 0
fi
