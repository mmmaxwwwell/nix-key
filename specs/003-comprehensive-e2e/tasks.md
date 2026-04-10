# Tasks: 003-comprehensive-e2e

**Approach**: Clean-slate E2E after 002 fixes. MCP exploration with inline regression tests — every bug fix gets a test before the task is marked done. Sequential execution.

**CRITICAL**: For every app bug found and fixed during MCP exploration (Phases 2-4), the agent MUST write a UI Automator regression test in `android/app/src/androidTest/java/com/nixkey/e2e/regression/` BEFORE marking the task done. Use `NixKeyE2EHelper` methods. A fix without a test is not done. Phase 5-6 scripted tasks do NOT generate separate regression tests — the scripted tests themselves ARE the regression tests (they run in CI via `e2e.yml`).

---

## Phase 1: Infrastructure Verification

- [ ] T300 Verify emulator, APK, MCP tools, and headscale infrastructure. (1) Boot emulator via `start-emulator`, verify `sys.boot_completed` = 1. (2) Build debug APK via `make android-apk`, install via `adb install`. (3) Launch app, verify no crash. (4) Verify MCP Screenshot returns valid PNG, DumpHierarchy returns XML with UI elements. (5) Run `test/e2e/setup.sh` — verify headscale starts, host tailscale joins mesh, nix-key daemon exposes SSH_AUTH_SOCK. (6) Verify deep link pairing works: `adb shell am start -a android.intent.action.VIEW -d "nix-key://pair?payload=..."`. (7) Verify fingerprint simulation: `adb -e emu finger touch 1`. [FR-500, FR-501, FR-502, FR-503]
  **Done when**: Emulator running, APK installed, MCP tools functional, headscale mesh operational, deep link + fingerprint simulation both work.

- [ ] T301 Set up second host and Jaeger collector for extended testing. (1) In headscale, create a second pre-auth key for a second host node. (2) Start a second `nix-key daemon` with a separate config directory (`/tmp/nix-key-host2/config/`), separate socket paths (`socketPath: /tmp/nix-key-host2/agent.sock`, `controlSocketPath: /tmp/nix-key-host2/control.sock`), separate state dir (`/tmp/nix-key-host2/state/`), and the second pre-auth key. This daemon runs alongside the primary one — each has its own SSH_AUTH_SOCK and device registry. (3) Start Jaeger all-in-one (from `nix/jaeger.nix` or `nix develop`) with OTLP receiver on port 4317. (4) Export `OTEL_ENDPOINT=127.0.0.1:4317`, `HOST2_NAME=host2`, `HOST2_IP=<host2 tailscale IP>`, `HOST2_SOCKET=/tmp/nix-key-host2/agent.sock` to `test/e2e/.state/env`. (5) Update `test/e2e/teardown.sh` to kill the second daemon and Jaeger, and clean up `/tmp/nix-key-host2/`. [FR-504]
  **Done when**: Second daemon running with its own sockets. `nix-key --config /tmp/nix-key-host2/config/config.json status` responds. Jaeger accepting traces on :4317 (verify: `curl -s http://localhost:16686/api/services` returns JSON). `.state/env` has `OTEL_ENDPOINT`, `HOST2_NAME`, `HOST2_IP`, `HOST2_SOCKET`. Teardown cleans up both daemons and Jaeger.

## Phase 2: Screen Exploration (MCP — one per screen)

- [ ] T302 Validate TailscaleAuthScreen [needs: mcp-android, e2e-loop] [FR-510, SC-300]
  Explore TailscaleAuth on live emulator. Verify against UI_FLOW.md § Tailscale Auth Screen: logo, title, auth key field (monospace, paste-friendly), "Connect" button, OR divider, "Sign in with Tailscale" button, connection indicator (yellow/"Connecting" during auth). Test: valid auth key accepted, invalid rejected with exact error "Invalid auth key format", `hskey-auth-` prefix accepted (BUG-010 regression), empty field rejected, whitespace rejected, connection indicator transitions, back exits app. Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for TailscaleAuth. Every fixed bug has a regression test in `e2e/regression/`.

- [ ] T303 Validate ServerListScreen [needs: mcp-android, e2e-loop] [FR-510, FR-512, SC-300]
  Explore ServerList. Verify: title bar, Tailnet indicator (green/yellow/red per FR-110), gear icon, **loading state** (CircularProgressIndicator during initial host fetch — new from 002), empty state ("No paired hosts yet"), "Scan QR Code" button. After pairing: host row with name, IP, connection status dot. Test: gear → Settings navigation, QR button → Pairing navigation, pull-to-refresh, back exits app. Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for ServerList. Loading state verified (CircularProgressIndicator appears). Every fixed bug has a regression test.

- [ ] T304 Validate PairingScreen [needs: mcp-android, e2e-loop] [FR-510, SC-300]
  Explore Pairing. Verify: deep link entry (`nix-key://pair?payload=...`), Tailnet indicator, cancel button, loading states ("Scanning...", "Connecting to host...", "Waiting for host approval..." per FR-113), success/failure result screens. Test: valid deep link pair → host appears in ServerList (BUG-018 regression), invalid payload → "Invalid QR code" or "Not a nix-key pairing code", token replay → rejected, cancel → back to ServerList. If OTEL endpoint in payload, verify "Enable tracing?" prompt appears. Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for Pairing. Post-pairing navigation to ServerList confirmed. Every fixed bug has a regression test.

- [ ] T305 Validate KeyManagementScreen [needs: mcp-android, e2e-loop] [FR-510, FR-512, SC-300]
  Explore KeyManagement (navigate via host row on ServerList). Verify: **loading state** (CircularProgressIndicator during key fetch — new from 002), key list with lock/unlock indicators (FR-111), key type badges, fingerprints, created dates, FAB create button, empty state ("No keys yet"). Test: create key → appears in list (BUG-021 regression), tap key row → KeyDetail, FAB → KeyDetail create mode, back → ServerList. Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for KeyManagement. Loading state verified. Every fixed bug has a regression test.

- [ ] T306 Validate KeyDetailScreen [needs: mcp-android, e2e-loop] [FR-510, FR-512, SC-300]
  Explore KeyDetail in both create and view/edit modes. Verify: **loading state** (CircularProgressIndicator during detail fetch — new from 002), key name field, key type selector (Ed25519 default, ECDSA-P256), type info text per UI_FLOW.md, unlock policy picker (Password default), signing policy picker (Biometric default), Create button (create mode), fingerprint (view mode), export section (clipboard/share/QR — verify all 3 buttons exist; functional testing of export actions is in T312), delete button, save button (appears on change). Test: create Ed25519 key, edit name, change policies, back navigation. Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for KeyDetail create + view/edit modes. Loading state verified. Every fixed bug has a regression test.

- [ ] T307 Validate SettingsScreen [needs: mcp-android, e2e-loop] [FR-510, SC-300]
  Explore Settings (navigate via gear icon). Verify: Security section ("Allow key listing" toggle default on, "Default unlock policy" picker with options None/Biometric/Password/Biometric+Password, "Default signing policy" picker with options Always ask/Biometric/Password/Biometric+Password/Auto-approve — labels must match UI_FLOW.md exactly), Tracing section (toggle + OTEL endpoint field), Tailscale section (IP, tailnet name, "Re-authenticate" button), About section (version, build, licenses). Test: toggle key listing, change default policies, invalid OTEL endpoint → exact error "Invalid endpoint format (expected host:port)", back → ServerList. Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for Settings. All dropdown labels match UI_FLOW.md. Every fixed bug has a regression test.

- [ ] T308 Validate SignRequestDialog [needs: mcp-android, e2e-loop] [FR-510, SC-300]
  Trigger a sign request from host (`ssh-keygen -Y sign` through SSH_AUTH_SOCK) and observe the dialog via MCP. Verify: dialog title "Sign Request", host name, key name, data hash (truncated SHA256), "Approve" button, "Deny" button. Test: tap Approve → biometric prompt → fingerprint simulation → SSH exits 0, tap Deny → SSH fails with SSH_AGENT_FAILURE. Also test via `nix-key://test-sign` deep link (BUG-015 regression). Write regression test for each bug fixed.
  **Done when**: findings.json has pass/fail for SignRequestDialog. Approve + deny both tested. Every fixed bug has a regression test.

## Phase 3: Flow Validation + Error Paths + Persistence (MCP)

- [ ] T309 Validate all navigation flows [needs: mcp-android, e2e-loop] [FR-511, SC-301]
  Exercise every navigation edge from UI_FLOW.md flowchart: first-launch (TailscaleAuth → ServerList, back stack cleared), ServerList → Pairing → ServerList, ServerList → KeyManagement → KeyDetail → KeyManagement → ServerList, ServerList → Settings → ServerList. Verify back navigation at every step. Write regression test for any broken navigation.
  **Done when**: findings.json has pass/fail per navigation flow. Every edge exercised. Every fixed bug has a regression test.

- [ ] T310 Validate error paths from Field Validation Reference Table [needs: mcp-android, e2e-loop] [FR-550, SC-307]
  Enter every invalid input from UI_FLOW.md Field Validation Reference Table and verify exact error message text:
  - TailscaleAuth: invalid key → `"Invalid auth key format"`
  - KeyDetail: 65-char name → `"Name must be 1-64 characters (letters, numbers, hyphens, underscores)"`
  - KeyDetail: duplicate name → `"A key with this name already exists"` (BUG-020 regression)
  - Settings: invalid OTEL → `"Invalid endpoint format (expected host:port)"`
  - Pairing: malformed deep link → `"Invalid QR code"` or `"Not a nix-key pairing code"`
  Write regression test for each error path.
  **Done when**: findings.json has pass/fail for every error message. All messages match spec exactly. Every fixed bug has a regression test.

- [ ] T311 Validate persistence across force-stop/restart [needs: mcp-android, e2e-loop] [FR-551, FR-552, SC-308]
  Create state (paired host + key), force-stop app (`adb shell am force-stop com.nixkey`), restart, verify: host still in ServerList, key still in KeyManagement, key shows locked state (unlock resets per spec). Write regression test for persistence.
  **Done when**: findings.json confirms state survives restart. Key shows locked. Regression test written.

## Phase 4: Extended Function Exploration (MCP)

- [ ] T312 Validate key export functions [needs: mcp-android, e2e-loop] [FR-520, SC-300]
  Navigate to existing key's detail. Test: (a) tap "Copy to Clipboard" → snackbar "Copied to clipboard" appears, (b) tap "Share" → system share sheet appears, dismiss, (c) tap "Show QR Code" → QR overlay displayed with dismiss button. Write regression test for each export method.
  **Done when**: findings.json has pass/fail for all 3 export methods. Regression tests written.

- [ ] T313 Validate security warning dialogs [needs: mcp-android, e2e-loop] [FR-521, SC-300]
  On KeyDetail: (a) change signing policy to "Auto-approve" → assert dialog with exact title "Security Warning" and body "Auto-approve allows sign requests to be processed without your confirmation. Any host with a valid mTLS certificate can trigger signing operations silently. Are you sure?" and buttons "Cancel" / "Enable Auto-Approve". Tap Cancel → policy unchanged. Tap again, tap "Enable Auto-Approve" → policy changed. (b) Change unlock policy to "None" → assert dialog with exact body "Disabling unlock means this key's material will be decrypted into memory automatically when the app starts. No biometric or password prompt will be required before signing operations can proceed." and buttons "Cancel" / "Disable Unlock". Write regression tests.
  **Done when**: findings.json confirms both dialogs with exact text. Cancel and enable buttons work. Regression tests written.

- [ ] T314 Validate connection state transitions [needs: mcp-android, e2e-loop] [FR-522, SC-300]
  (a) Verify Tailnet indicator green/"Connected" on ServerList via MCP Screenshot/DumpHierarchy. (b) Kill the host's tailscaled process (`sudo kill $(pgrep tailscaled)` or `systemctl stop tailscaled`) — this tests the phone detecting that the host is unreachable, which is the production failure mode. Do NOT kill headscale (that tests control plane loss, a different scenario). (c) Wait up to 30s for Tailnet indicator to change to red/"Disconnected". (d) Restart tailscaled (`systemctl start tailscaled` + rejoin headscale). (e) Wait up to 60s for indicator to return to green/"Connected". (f) Verify per-host connection status dots updated (host dot should go red then green). Write regression test if transitions work; file bug if not.
  **Done when**: findings.json confirms Tailnet indicator transitions green→red→green. Per-host connection dot also transitions.

- [ ] T315 Validate long-press re-lock + lock/unlock button [needs: mcp-android, e2e-loop] [FR-523, FR-524, SC-300]
  (a) Unlock a key (trigger sign, approve). (b) On KeyManagement, long-press unlocked key → "Re-lock key" context menu → tap → key shows locked. (c) On KeyDetail, verify "Unlock Key" button → tap → auth prompt → complete → button changes to "Lock Key" → tap → key re-locked. Write regression tests.
  **Done when**: findings.json confirms long-press and button both work. Regression tests written.

- [ ] T316 Validate multiple paired hosts [needs: mcp-android, e2e-loop] [FR-524, SC-300]
  Using second host from T301: (a) pair with second host via deep link, (b) ServerList shows both hosts with names, IPs, connection dots, (c) tap each host → correct KeyManagement. Write regression test if both hosts visible.
  **Done when**: findings.json confirms both hosts visible and navigable. Regression test written.

- [ ] T317 Validate ECDSA-P256 key creation path [needs: mcp-android, e2e-loop] [FR-525, SC-300]
  (a) KeyDetail create mode → select ECDSA-P256 → info text "Hardware-backed via Android Keystore (TEE/StrongBox)" appears, (b) create key, (c) KeyManagement shows key with "ECDSA-P256" badge, (d) KeyDetail shows fingerprint + export. Write regression test.
  **Done when**: findings.json confirms P256 creation with correct info text and badge. Regression test written.

## Phase 5: Cross-System + CLI Tests (Scripted)

- [ ] T318 Add multi-key scenario to `test/e2e/android_e2e_test.sh`. Create Ed25519 + ECDSA-P256 keys, verify both in `ssh-add -L`, sign with each. [FR-534, SC-304]
  **Done when**: Script creates both key types, `ssh-add -L` lists both, signing with each succeeds.

- [ ] T319 Add device revocation scenario. `nix-key revoke <device>`, verify `nix-key devices` no longer lists it, verify sign request fails. [FR-535, SC-305]
  **Done when**: Script runs `nix-key revoke`, device gone from output, sign fails with SSH_AGENT_FAILURE.

- [ ] T320 Add concurrent sign scenario. The shell script triggers 3 `ssh-keygen -Y sign` commands in background from the host. To approve each on the emulator, the script calls `adb shell am instrument -w -e class com.nixkey.e2e.NixKeyE2EHelper#approveSignRequest com.nixkey.test/androidx.test.runner.AndroidJUnitRunner` three times in sequence (or writes a small instrumented test class `ConcurrentSignHelper` that calls `NixKeyE2EHelper.approveSignRequest(30000)` three times). After all 3 approvals, wait for all background SSH PIDs and verify exit code 0 for each. [FR-533, SC-303]
  **Done when**: 3 concurrent signs all exit 0. Script orchestrates host-side signs via background processes and emulator-side approvals via `adb shell am instrument`. All 3 SSH processes exit 0.

- [ ] T321 Add sign timeout scenario. (1) Set `signTimeout` to 5s in the daemon config (update config.json, restart daemon). (2) Trigger `ssh-keygen -Y sign` from the host. (3) Do NOT interact with the emulator — no `approveSignRequest`, no `denySignRequest`. (4) Wait 10s. (5) Verify the SSH operation failed (non-zero exit code). (6) Optionally verify via `adb shell am instrument` or MCP that the sign dialog is no longer visible on the emulator. (7) Restore original `signTimeout` and restart daemon. [FR-532, SC-303]
  **Done when**: SSH fails after timeout with non-zero exit code. No emulator interaction occurred during the timeout window.

- [ ] T322 Add CLI exercise scenario. Requires 002-deficiency-fixes T054-fix (STATUS column) to be complete. With paired device: `nix-key status` → output contains `"running"`, `nix-key devices` → output contains column headers `"STATUS"` and `"SOURCE"` (both must be present — STATUS is from T054-fix, SOURCE is from original implementation), `nix-key export <fingerprint>` → output starts with `ssh-ed25519` or `ecdsa-sha2-nistp256`, `nix-key test <device>` → output contains `"success"`. [FR-540, FR-541, FR-542, FR-543, SC-306]
  **Done when**: All 4 CLI commands produce expected output. Script asserts: `"running"` in status output, `"STATUS"` in devices header, `"SOURCE"` in devices header, `"ssh-ed25519"` or `"ecdsa-sha2-nistp256"` in export output, `"success"` in test output.

## Phase 6: Resilience + OTEL (Scripted)

- [ ] T323 Add daemon restart resilience test. Verify signing works → kill/restart daemon → verify signing works again. [FR-560, SC-310]
  **Done when**: Sign succeeds after daemon restart.

- [ ] T324 Add app restart resilience test. Verify signing → `adb shell am force-stop` → relaunch → poll `nix-key test <device>` until reachable → verify signing works. [FR-561, SC-310]
  **Done when**: Sign succeeds after app restart. Script polls until device reachable.

- [ ] T325 Add OTEL trace verification. (1) Pair emulator app with OTEL endpoint from `.state/env` included in the QR deep link payload. (2) Trigger and approve a sign request. (3) Wait 5s for trace export. (4) Query Jaeger v2 API: `curl -s "http://localhost:16686/api/traces?service=nix-key&limit=1"`. (5) Assert the response JSON contains at least one trace. (6) Assert the trace's `processes` map contains both `nix-key` and `nix-key-phone` service names. (7) Assert at least one phone span has a `CHILD_OF` reference whose `traceID` matches a host span (traceparent propagation). [FR-563, SC-309]
  **Done when**: `curl http://localhost:16686/api/traces?service=nix-key&limit=1` returns JSON with a trace containing both `nix-key` and `nix-key-phone` services. Phone spans have `CHILD_OF` references to host spans.

## Phase 7: Final Validation

- [ ] T-validate-003 Run full validation: (1) `make validate` — exit 0, (2) `./gradlew connectedDebugAndroidTest` — all regression tests pass, (3) `test/e2e/android_e2e_test.sh` — all scenarios pass, (4) verify regression test count > 0 (no unprotected fixes). [SC-311, SC-312]
  **Done when**: All commands exit 0. Every bug fixed during this feature has a regression test. No existing tests broken.

---

## Execution Order

```
T300 → T301 → T302 → T303 → T304 → T305 → T306 → T307 → T308 → T309 → T310 → T311 → T312 → T313 → T314 → T315 → T316 → T317 → T318 → T319 → T320 → T321 → T322 → T323 → T324 → T325 → T-validate-003
```

Strictly sequential. Each task completes before the next begins.

## Phase Dependencies

```
Phase 1 (T300-T301) → Phase 2 (T302-T308) → Phase 3 (T309-T311) → Phase 4 (T312-T317) → Phase 5 (T318-T322) → Phase 6 (T323-T325) → Phase 7 (T-validate-003)
```
