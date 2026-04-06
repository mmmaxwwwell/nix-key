#!/usr/bin/env bash
# scripts/setup-branch-protection.sh
#
# Configures GitHub branch protection rules for nix-key.
# Requires: gh CLI authenticated with admin access to the repository.
#
# Usage: ./scripts/setup-branch-protection.sh [OWNER/REPO]
#   If OWNER/REPO is omitted, auto-detects from git remote.

set -euo pipefail

REPO="${1:-}"
if [ -z "$REPO" ]; then
  REPO=$(gh repo view --json nameWithOwner -q '.nameWithOwner' 2>/dev/null) || {
    echo "Error: could not detect repository. Pass OWNER/REPO as argument." >&2
    exit 1
  }
fi

echo "Configuring branch protection for: ${REPO}"

# ---------- main branch ----------
# Requires: all CI jobs + security + E2E green
echo ""
echo "Setting up 'main' branch protection..."
gh api -X PUT "repos/${REPO}/branches/main/protection" \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Lint",
      "Test Host",
      "Test Android",
      "Security Scan",
      "CI Summary",
      "Android Emulator E2E"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "required_linear_history": false,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
echo "  -> main branch protection configured"

# ---------- develop branch ----------
# Requires: lint + test-host + test-android green
echo ""
echo "Setting up 'develop' branch protection..."
gh api -X PUT "repos/${REPO}/branches/develop/protection" \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Lint",
      "Test Host",
      "Test Android"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "required_linear_history": false,
  "allow_force_pushes": false,
  "allow_deletions": false
}
EOF
echo "  -> develop branch protection configured"

echo ""
echo "Branch protection setup complete."
echo ""
echo "Required status checks:"
echo "  main:    Lint, Test Host, Test Android, Security Scan, CI Summary, Android Emulator E2E"
echo "  develop: Lint, Test Host, Test Android"
echo ""
echo "release-please is configured in .github/workflows/release.yml to auto-create"
echo "release PRs when develop is merged to main (triggered by push to main)."
