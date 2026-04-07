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

## E2E Bug Fix Pass #3 (7 bugs)

- **Dropdown label suffixes in KeyDetailScreen (BUG-001/002):** The SettingsScreen `settingsLabel()` was fixed in pass #2, but the KeyDetailScreen still had `displayLabel()` with "(auto-unlock)" and "only" suffixes. Both screens must use matching spec-compliant labels.
- **TailscaleAuth back navigation via re-authenticate (BUG-003):** `popUpTo(Routes.SERVER_LIST)` in NavGraph didn't fully clear the back stack when re-authenticating from Settings. Changed to `popUpTo(navController.graph.id) { inclusive = true }` to clear the entire graph, ensuring Back exits the app.
- **OTEL inline validation (BUG-004/007):** Moved validation from blur-only to inline in `setOtelEndpoint()` — error is now computed on every keystroke. This ensures the error persists across navigation. Invalid values are still never saved to SharedPreferences.
- **Open source licenses clickability (BUG-005):** `Modifier.clickable` on a Text composable doesn't always propagate `clickable=true` to the native accessibility tree. Changed to `TextButton` which has proper Material accessibility semantics. Also wired `onLicenses` callback in NavGraph.
- **Pairing error Cancel button (BUG-006):** The top bar "Cancel" button was always visible including on the error result screen. Now hidden when `phase == ERROR`, leaving only the "Done" button per spec.

## E2E Bug Fix Pass #4 (7 bugs)

- **Dropdown default indicators (BUG-001/002):** `settingsLabel()` labels matched spec names but were missing the "(default)" suffix on the default option. Per UI_FLOW.md, `UnlockPolicy.PASSWORD` should show "Password (default)" and `ConfirmationPolicy.BIOMETRIC` should show "Biometric (default)". Only applies to `settingsLabel()` (not `displayLabel()` in KeyDetailScreen, which is per-key not a default choice).
- **TailscaleAuth back navigation (BUG-003):** `popUpTo(navController.graph.id) { inclusive = true }` doesn't reliably clear the back stack when re-authenticating from Settings. Changed to `popUpTo(Routes.SERVER_LIST) { inclusive = true }` with `launchSingleTop = true` for explicit back stack clearing.
- **OTEL endpoint persistence vs validation (BUG-004/007):** Previous fix prevented invalid values from being persisted, but this caused the invalid text to disappear on navigation (field reverted to old valid value). The spec requires either non-persistence OR persistence with validation on load. Changed to always persist the endpoint value; `loadSettings()` already validates on init and shows the error, so the error now survives navigation.
- **Open source licenses clickability (BUG-005):** `TextButton` wrapping Text didn't expose `clickable=true` on the Text node in the view hierarchy for UI test frameworks. Replaced with `Text` + `Modifier.clickable` which sets the click handler directly on the text element.
- **Pairing error unused onRetry (BUG-006):** ErrorContent had an unused `onRetry` parameter from when Cancel/Try Again buttons existed. Cleaned up to only accept `onDone` callback.

## E2E Bug Fix Pass #5 (7 bugs)

- **BUG-001/002/006 already fixed:** Labels in `settingsLabel()` and ErrorContent "Done" button were correct from pass #4. No changes needed.
- **TailscaleAuth back navigation (BUG-003):** `popUpTo(Routes.SERVER_LIST) { inclusive = true }` (pass #4 approach) still doesn't clear the back stack because Compose Navigation won't pop the graph's start destination via route string. Reverted to `popUpTo(navController.graph.id) { inclusive = true }` which pops the entire nav graph, leaving TAILSCALE_AUTH as the only entry so BackHandler's `finishAffinity()` can fire.
- **OTEL endpoint validation (BUG-004/007):** The "persist always + validate on load" approach from pass #4 still had issues (BUG-007: invalid endpoints used for OTEL export). Changed to **never persist invalid values** — only save to `settingsRepository` when `isValidHostPort()` passes. This fixes both bugs: invalid values can't be used (BUG-007), and on screen re-entry the last valid value loads without error (BUG-004 becomes N/A).
- **Open source licenses clickability (BUG-005):** `Modifier.clickable(onClick = ...)` without `role` parameter doesn't expose `clickable=true` to UI Automator. Added `role = Role.Button` to `Modifier.clickable()` call plus `fillMaxWidth()` and vertical padding for a proper tap target.

## E2E Bug Fix Pass #6 (7 bugs)

- **Settings toggle accessibility (BUG-008):** `Switch` composable inside a `Row` was not individually exposed in the accessibility tree — the `ScrollView` parent merged all text. Fix: make the entire `Row` a `clickable(role = Role.Switch)` with `semantics { toggleableState, stateDescription, contentDescription }`, and set `Switch(onCheckedChange = null)` with `clearAndSetSemantics {}` to prevent duplicate nodes.
- **OTEL endpoint data loss (BUG-012):** `setOtelEndpoint()` was saving to repository on every keystroke when `error == null`, which includes empty string. Typing cleared the field intermediately, saving empty and losing the valid value. Fix: remove persistence from `setOtelEndpoint()` entirely. Only save in `validateOtelEndpoint()` (on blur). If invalid on blur, revert to last saved value from repository.
- **Internal error exposed (BUG-013):** `TailscaleManager.start()` throws `IllegalStateException("TailscaleManager is already running")` which was passed verbatim as `"Connection failed: ${e.message}"`. Fix: add `userFriendlyError()` helper that maps known internal exceptions to user-facing messages. Applied to both `connectWithAuthKey()` and `connectWithOAuth()`.
- **Error text not in accessibility tree (BUG-014):** Error `Text` composables across TailscaleAuth (error phase + input validation), PairingScreen (error content), and Settings (OTEL error) were not announced by screen readers. Fix: add `Modifier.semantics { liveRegion = LiveRegionMode.Polite }` to all error text elements.
- **Auth key special character validation (BUG-015):** `isValidAuthKeyFormat()` only checked for prefix and whitespace. Keys like `tskey-auth-<script>alert(1)</script>` were accepted. Fix: replace with regex `^tskey-(auth-)?[a-zA-Z0-9-]+$` to enforce alphanumeric-and-dash character set.
- **Deep link reprocessing on rotation (BUG-016/017):** `intent.data` was not cleared after consumption. Activity recreation on rotation re-ran `extractPairPayload(intent)` and re-triggered navigation. Fix: guard extraction with `savedInstanceState == null` in `onCreate`, and clear `intent.data` + call `setIntent(Intent())` after extraction in both `onCreate` and `onNewIntent`.

## E2E Bug Fix Pass #7 (5 bugs)

- **Settings toggle accessibility (BUG-008):** Pass #6's `clickable(role = Role.Switch)` approach still didn't expose toggle state properly in the Android accessibility tree. Fix: replaced `Modifier.clickable(role = Role.Switch)` with `Modifier.toggleable(value = checked, role = Role.Switch, onValueChange = onCheckedChange)` which uses Compose's built-in toggleable semantics to properly expose checked/unchecked state. Removed redundant `toggleableState` from the semantics block since `toggleable()` handles it.
- **Pairing error text not in accessibility tree (BUG-014):** The `liveRegion` semantics alone was insufficient for the "Pairing Failed" and error description text to appear in the accessibility tree. Fix: added explicit `contentDescription` and `heading()` semantics to force the text nodes to be individually accessible.
- **OTEL endpoint not saved on back navigation (BUG-018):** `setOtelEndpoint()` only updated UI state; persistence was deferred to `validateOtelEndpoint()` which required a blur event that doesn't fire on Back navigation. Fix: persist valid endpoints immediately in `setOtelEndpoint()` (matching the save-on-change behavior of toggles and dropdowns). This ensures the value survives navigation regardless of focus state.
- **Rotation crash / ForegroundServiceDidNotStartInTimeException (BUG-019):** `MainActivity.onStop()` called `GrpcServerService.stopService()` and `onStart()` called `startService()`. On rotation, the rapid stop→start cycle caused a race where `startForeground()` wasn't called within the 10-second deadline. Fix: check `isChangingConfigurations` in `onStop()` to skip stopping the service, and track the flag via `wasChangingConfigurations` to skip redundant `startService()` in the subsequent `onStart()`.
- **ServerList static text not in accessibility tree (BUG-020):** The empty state texts ("No paired hosts yet", "Scan a QR code to pair.") and connection status indicator were not exposed. Fix: added `Modifier.semantics { heading(); contentDescription = ... }` to the heading text and `contentDescription` to the helper text. Added `semantics(mergeDescendants = true) { contentDescription = "Connection status: ..." }` to `TailnetIndicator`.
