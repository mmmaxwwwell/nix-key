# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## Pre-implementation (from failed attempt 1)

- **NEVER write orchestration code for MCP E2E tasks.** The first attempt produced ~6000 lines of shell scripts (scenario runners, prompt templates, report libraries) that duplicated what the runner already provides. Agents must use MCP tools directly against the live emulator.
- **Validate with real infrastructure, not synthetic data.** The first attempt's "validation" tasks tested the framework with fake data instead of booting an emulator. Every validation must touch the real app.
- **The parallel runner handles emulator boot, APK build+install, MCP server lifecycle.** Tasks just need `[needs: mcp-android, e2e-loop]` — the runner does the rest.

## T002 — Test bypass mechanisms

- **Deep link pairing works on debug APK only.** The intent filter is in `src/debug/AndroidManifest.xml` (not main). Camera permission dialog still appears on first deep link — dismiss it or grant camera before testing pairing flow.
- **Fingerprint simulation + AUTO_APPROVE are two independent bypass paths.** `adb -e emu finger touch 1` works with `BiometricPrompt` (BIOMETRIC policy), while `AUTO_APPROVE` policy skips prompts entirely. For E2E, prefer AUTO_APPROVE + UnlockPolicy.NONE to avoid any biometric interaction.
- **StrongBox fallback is automatic — no debug flag needed.** Emulators have no StrongBox; KeyManager.kt catches the exception and retries without it. No special configuration for E2E.

## T003 — Headscale/tailscale/daemon infrastructure

- **`tailscale up` may fail if chained immediately after `tailscaled &`.** The daemon needs ~3s to initialize its IPN state machine. The E2E script's `sleep 2` is borderline; running `tailscale up` as a separate step (not `&&`-chained) is more reliable.
- **Headscale 0.28+ uses numeric user IDs for `--user` flag.** The `users list -o json` + python3 extraction pattern in `android_e2e_test.sh` is required; passing the username string no longer works for `preauthkeys create`.
- **Agent socket responds immediately after creation.** `ssh-add -L` returns "no identities" (exit 1) which is correct with no paired devices — the protocol is functional.

## T001 — Infrastructure verification

- **Emulator boots in ~40s with KVM.** `start-emulator --no-wait` + polling `sys.boot_completed` is the pattern. AVD creation via `avdmanager` may fail in Nix (path issues); the script's manual fallback handles this.
- **"System UI isn't responding" dialog appears on first boot** — dismiss it before UI assertions. It's the system launcher, not the app.
- **MCP Screenshot/DumpHierarchy map to `adb exec-out screencap -p` and `uiautomator dump`** respectively. Both work on the emulator out of the box.

## E2E Bug Fix Pass (12 bugs)

- **Auth key validation (BUG-001):** `TailscaleAuthViewModel.connectWithAuthKey()` only checked for empty string. Keys must start with `tskey-auth-` or `tskey-` prefix.
- **Missing Settings sections (BUG-002/003):** Added Tailscale section (IP, tailnet name, re-authenticate) and About section (version, build, licenses). Required injecting `TailscaleManager` into `SettingsViewModel`.
- **Java internals in error messages (BUG-004/005):** `PairingViewModel.onQrScanned()` passed raw `e.message` to UI. Fixed with static user-friendly message; internal details still logged via Timber.
- **OTEL validation (BUG-006):** Added `onFocusChanged` blur-triggered validation for host:port format in SettingsScreen.
- **TailscaleAuth missing UI (BUG-007/008):** Added app logo via `R.mipmap.ic_launcher` and connection indicator using `LocalTailnetConnectionState`.
- **Cancel button broken (BUG-009):** ErrorContent's `onBack` didn't call `viewModel.resetState()` first. Fixed by wrapping the callback.
- **Empty state (BUG-010/011):** Added illustration (launcher icon), fixed subtitle to match spec ("Scan a QR code to pair.").
- **Back arrow vs Cancel (BUG-012):** Replaced `IconButton` with `TextButton("Cancel")` in pairing screen `TopAppBar.navigationIcon`.
- **Material icons:** Only default set available without extended icons dependency. Use `R.mipmap.ic_launcher` as fallback illustration.

## E2E Bug Fix Pass #2 (7 bugs)

- **Dropdown label suffixes (BUG-001/002):** `settingsLabel()` extensions had non-spec suffixes like "(auto-unlock)" and "only". Labels must exactly match UI_FLOW.md Field Validation Reference Table.
- **TailscaleAuth back navigation (BUG-003):** No `BackHandler` on auth screen let NavHost pop to ServerList, bypassing auth. Fixed by adding `BackHandler { finishAffinity() }` to exit the app.
- **OTEL validation persistence (BUG-004/007):** Validation error was only set on blur but not restored on screen re-entry. Fixed by validating in `loadSettings()`. Also gated persistence — invalid values are no longer saved to SharedPreferences.
- **Open source licenses not clickable (BUG-005):** The `Text` composable was missing `Modifier.clickable()`. Added an `onLicenses` callback parameter.
- **Pairing error buttons (BUG-006):** Error screen showed Cancel/Try Again per original implementation. Spec requires a single "Done" button navigating to Server List.
