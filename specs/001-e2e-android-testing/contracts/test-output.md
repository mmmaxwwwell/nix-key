# Contract: E2E Test Output Format

**Feature**: 001-e2e-android-testing  
**Date**: 2026-04-05

## Output Location

```
test-logs/e2e/
├── summary.json          # Aggregate results (compatible with ci-summary.sh)
├── screenshots/          # Failure screenshots (PNG, named by scenario ID)
└── hierarchies/          # UI hierarchy dumps on failure (XML)
```

## summary.json Schema

Compatible with the existing `cmd/test-reporter` output format used by `scripts/ci-summary.sh`.

```json
{
  "suite": "e2e",
  "started_at": "2026-04-05T12:00:00Z",
  "completed_at": "2026-04-05T12:45:00Z",
  "mode": "ci",
  "infrastructure": {
    "emulator_api": 34,
    "headscale_version": "0.23.x",
    "kvm_available": true
  },
  "summary": {
    "total": 25,
    "passed": 23,
    "failed": 1,
    "skipped": 1,
    "duration_ms": 2700000
  },
  "scenarios": [
    {
      "id": "US1-AS1",
      "user_story": 1,
      "title": "Emulator boots and agents begin navigating within 5 minutes",
      "status": "passed",
      "duration_ms": 180000,
      "screenshots": [],
      "failure_detail": null
    },
    {
      "id": "US4-AS1",
      "user_story": 4,
      "title": "Invalid auth key shows validation error",
      "status": "failed",
      "duration_ms": 5000,
      "screenshots": ["screenshots/US4-AS1-failure.png"],
      "failure_detail": "Expected 'Invalid auth key format' but found 'Error'"
    }
  ]
}
```

## ci-summary.sh Integration

The E2E job entry in `ci-summary.json` follows the existing format:

```json
{
  "name": "e2e",
  "result": "failure",
  "pass": 23,
  "fail": 1,
  "skip": 1,
  "duration": "45m00s",
  "failures": [
    "US4-AS1: Expected 'Invalid auth key format' but found 'Error'"
  ]
}
```

## Explore-Fix-Verify Output (Local Mode Only)

When running in local mode with the explore-fix-verify loop, additional output is produced:

```
test-logs/e2e/
├── bugs/
│   ├── BUG-001.json      # Individual bug reports
│   └─��� BUG-002.json
├── iterations/
│   ├── iter-001.json     # Per-iteration summary
│   └── iter-002.json
└── supervisor/
    └── review-001.json   # Supervisor progress reviews
```

### Bug Report Format

```json
{
  "id": "BUG-001",
  "scenario_id": "US2-AS3",
  "description": "Key name edit not persisting after save",
  "expected": "Updated name 'my-key-renamed' visible in key list",
  "actual": "Original name 'my-key' still shown after save and navigate back",
  "severity": "major",
  "status": "verified",
  "screenshot": "screenshots/BUG-001-found.png",
  "fix_files": ["android/app/src/main/java/com/nixkey/data/HostRepository.kt"],
  "fix_iteration": 2,
  "found_iteration": 1,
  "verified_iteration": 3
}
```
