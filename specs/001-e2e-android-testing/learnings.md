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

## T001 — Infrastructure verification

- **Emulator boots in ~40s with KVM.** `start-emulator --no-wait` + polling `sys.boot_completed` is the pattern. AVD creation via `avdmanager` may fail in Nix (path issues); the script's manual fallback handles this.
- **"System UI isn't responding" dialog appears on first boot** — dismiss it before UI assertions. It's the system launcher, not the app.
- **MCP Screenshot/DumpHierarchy map to `adb exec-out screencap -p` and `uiautomator dump`** respectively. Both work on the emulator out of the box.
