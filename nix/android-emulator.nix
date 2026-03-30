# nix/android-emulator.nix
#
# Android emulator infrastructure for E2E testing.
#
# Provides a headless Android emulator (API 34, x86_64) with:
#   - Swiftshader GPU (software rendering, no host GPU required)
#   - KVM acceleration
#   - 2GB RAM AVD configuration
#
# Exports:
#   emulatorSdk       - Android SDK with emulator + system image
#   start-emulator    - Script that boots emulator and waits for boot
#   avdHome           - Path to pre-created AVD
#
# Usage:
#   start-emulator              # boots emulator, waits for sys.boot_completed
#   start-emulator --no-wait    # boots emulator without waiting
#   start-emulator --kill       # kills running emulator
#
{ pkgs, lib, }:

let
  # --- Android SDK with emulator + system image ---
  emulatorComposition = pkgs.androidenv.composeAndroidPackages {
    platformVersions = [ "34" ];
    buildToolsVersions = [ "34.0.0" ];
    platformToolsVersion = "35.0.2";
    includeNDK = false;
    includeSources = false;
    includeEmulator = true;
    includeSystemImages = true;
    systemImageTypes = [ "google_apis" ];
    abiVersions = [ "x86_64" ];
    extraLicenses = [
      "android-sdk-license"
      "android-sdk-preview-license"
      "android-googletv-license"
      "google-gdk-license"
    ];
  };

  emulatorSdk = emulatorComposition.androidsdk;
  emulatorHome = "${emulatorSdk}/libexec/android-sdk";

  # --- Helper script: start-emulator ---
  start-emulator = pkgs.writeShellScriptBin "start-emulator" ''
    set -euo pipefail

    ANDROID_HOME="${emulatorHome}"
    export ANDROID_HOME
    export ANDROID_SDK_ROOT="$ANDROID_HOME"
    export PATH="$ANDROID_HOME/emulator:$ANDROID_HOME/platform-tools:$PATH"

    AVD_NAME="nix-key-test"
    BOOT_TIMEOUT=120
    ACTION="start"

    for arg in "$@"; do
      case "$arg" in
        --no-wait) ACTION="start-no-wait" ;;
        --kill)    ACTION="kill" ;;
        --help|-h)
          echo "Usage: start-emulator [--no-wait] [--kill] [--help]"
          echo ""
          echo "Options:"
          echo "  --no-wait  Start emulator without waiting for boot"
          echo "  --kill     Kill running emulator"
          echo "  --help     Show this help"
          exit 0
          ;;
        *)
          echo "Unknown argument: $arg" >&2
          exit 1
          ;;
      esac
    done

    kill_emulator() {
      echo "Killing emulator..."
      adb -s emulator-5554 emu kill 2>/dev/null || true
      # Wait for process to exit
      for i in $(seq 1 10); do
        if ! adb devices 2>/dev/null | ${pkgs.gnugrep}/bin/grep -q "emulator-5554"; then
          echo "Emulator stopped."
          return 0
        fi
        sleep 1
      done
      echo "Warning: emulator may still be running" >&2
      return 1
    }

    if [ "$ACTION" = "kill" ]; then
      kill_emulator
      exit $?
    fi

    # --- Create AVD if it doesn't exist ---
    AVD_DIR="''${ANDROID_USER_HOME:-$HOME/.android}/avd"
    mkdir -p "$AVD_DIR"

    if [ ! -d "$AVD_DIR/$AVD_NAME.avd" ]; then
      echo "Creating AVD: $AVD_NAME (API 34, x86_64, 2GB RAM)"

      # Find the system image path
      SYS_IMG="$ANDROID_HOME/system-images/android-34/google_apis/x86_64"
      if [ ! -d "$SYS_IMG" ]; then
        echo "ERROR: System image not found at $SYS_IMG" >&2
        echo "Available system images:" >&2
        ${pkgs.findutils}/bin/find "$ANDROID_HOME/system-images" -maxdepth 3 -type d 2>/dev/null || true
        exit 1
      fi

      # Create AVD using avdmanager
      echo "no" | "$ANDROID_HOME/cmdline-tools/latest/bin/avdmanager" create avd \
        --name "$AVD_NAME" \
        --package "system-images;android-34;google_apis;x86_64" \
        --device "pixel_6" \
        --force \
        2>&1 || {
          # Fallback: create AVD manually if avdmanager fails
          echo "avdmanager failed, creating AVD manually..."
          mkdir -p "$AVD_DIR/$AVD_NAME.avd"

          cat > "$AVD_DIR/$AVD_NAME.ini" <<AVDINI
    avd.ini.encoding=UTF-8
    path=$AVD_DIR/$AVD_NAME.avd
    path.rel=avd/$AVD_NAME.avd
    target=android-34
    AVDINI

          cat > "$AVD_DIR/$AVD_NAME.avd/config.ini" <<CFGINI
    AvdId=$AVD_NAME
    PlayStore.enabled=false
    abi.type=x86_64
    avd.ini.displayname=$AVD_NAME
    avd.ini.encoding=UTF-8
    disk.dataPartition.size=2048M
    hw.accelerator.isAccelerated=yes
    hw.cpu.arch=x86_64
    hw.cpu.ncore=2
    hw.gpu.enabled=yes
    hw.gpu.mode=swiftshader_indirect
    hw.keyboard=yes
    hw.lcd.density=420
    hw.lcd.height=2400
    hw.lcd.width=1080
    hw.ramSize=2048
    hw.sdCard.status=absent
    image.sysdir.1=$SYS_IMG/
    showDeviceFrame=no
    skin.dynamic=yes
    tag.display=Google APIs
    tag.id=google_apis
    vm.heapSize=576
    CFGINI
        }

      # Ensure hardware acceleration and RAM settings
      CONFIG_FILE="$AVD_DIR/$AVD_NAME.avd/config.ini"
      if [ -f "$CONFIG_FILE" ]; then
        # Set/override critical settings
        ${pkgs.gnused}/bin/sed -i \
          -e 's/^hw.ramSize=.*/hw.ramSize=2048/' \
          -e 's/^hw.gpu.mode=.*/hw.gpu.mode=swiftshader_indirect/' \
          -e 's/^hw.gpu.enabled=.*/hw.gpu.enabled=yes/' \
          "$CONFIG_FILE"

        # Add settings if not present
        ${pkgs.gnugrep}/bin/grep -q "^hw.ramSize=" "$CONFIG_FILE" || echo "hw.ramSize=2048" >> "$CONFIG_FILE"
        ${pkgs.gnugrep}/bin/grep -q "^hw.gpu.mode=" "$CONFIG_FILE" || echo "hw.gpu.mode=swiftshader_indirect" >> "$CONFIG_FILE"
        ${pkgs.gnugrep}/bin/grep -q "^hw.gpu.enabled=" "$CONFIG_FILE" || echo "hw.gpu.enabled=yes" >> "$CONFIG_FILE"
        ${pkgs.gnugrep}/bin/grep -q "^hw.accelerator.isAccelerated=" "$CONFIG_FILE" || echo "hw.accelerator.isAccelerated=yes" >> "$CONFIG_FILE"
      fi

      echo "AVD created: $AVD_NAME"
    else
      echo "AVD already exists: $AVD_NAME"
    fi

    # --- Check KVM availability ---
    if [ -w /dev/kvm ]; then
      echo "KVM: available"
      KVM_FLAG="-accel on"
    else
      echo "Warning: /dev/kvm not accessible, emulator will be slow" >&2
      KVM_FLAG="-accel off"
    fi

    # --- Start emulator ---
    echo "Starting emulator: $AVD_NAME"
    emulator @"$AVD_NAME" \
      -no-window \
      -no-audio \
      -no-boot-anim \
      -gpu swiftshader_indirect \
      $KVM_FLAG \
      -memory 2048 \
      -no-snapshot \
      -wipe-data \
      -verbose \
      &>"''${TMPDIR:-/tmp}/emulator-$AVD_NAME.log" &
    EMU_PID=$!

    echo "Emulator PID: $EMU_PID"
    echo "Log: ''${TMPDIR:-/tmp}/emulator-$AVD_NAME.log"

    if [ "$ACTION" = "start-no-wait" ]; then
      echo "Emulator started (not waiting for boot)."
      exit 0
    fi

    # --- Wait for boot_completed ---
    echo "Waiting for emulator boot (timeout: ''${BOOT_TIMEOUT}s)..."

    ELAPSED=0
    INTERVAL=5
    DEVICE_READY=false

    # First, wait for the adb device to appear
    while [ "$ELAPSED" -lt "$BOOT_TIMEOUT" ]; do
      # Check emulator process is still alive
      if ! kill -0 "$EMU_PID" 2>/dev/null; then
        echo "ERROR: Emulator process died. Check log:" >&2
        tail -20 "''${TMPDIR:-/tmp}/emulator-$AVD_NAME.log" >&2
        exit 1
      fi

      if adb devices 2>/dev/null | ${pkgs.gnugrep}/bin/grep -q "emulator-5554.*device"; then
        DEVICE_READY=true
        break
      fi

      sleep "$INTERVAL"
      ELAPSED=$((ELAPSED + INTERVAL))
      echo "  Waiting for adb device... (''${ELAPSED}s)"
    done

    if [ "$DEVICE_READY" = "false" ]; then
      echo "ERROR: Emulator device did not appear within ''${BOOT_TIMEOUT}s" >&2
      kill "$EMU_PID" 2>/dev/null || true
      tail -20 "''${TMPDIR:-/tmp}/emulator-$AVD_NAME.log" >&2
      exit 1
    fi

    echo "ADB device connected. Waiting for boot_completed..."

    # Now wait for sys.boot_completed
    while [ "$ELAPSED" -lt "$BOOT_TIMEOUT" ]; do
      if ! kill -0 "$EMU_PID" 2>/dev/null; then
        echo "ERROR: Emulator process died during boot. Check log:" >&2
        tail -20 "''${TMPDIR:-/tmp}/emulator-$AVD_NAME.log" >&2
        exit 1
      fi

      BOOT_PROP=$(adb -s emulator-5554 shell getprop sys.boot_completed 2>/dev/null || echo "")
      # Trim whitespace/carriage returns
      BOOT_PROP=$(echo "$BOOT_PROP" | ${pkgs.coreutils}/bin/tr -d '[:space:]')

      if [ "$BOOT_PROP" = "1" ]; then
        echo "Emulator booted successfully in ''${ELAPSED}s."
        echo "Device: $(adb -s emulator-5554 shell getprop ro.product.model 2>/dev/null || echo 'unknown')"
        echo "API:    $(adb -s emulator-5554 shell getprop ro.build.version.sdk 2>/dev/null || echo 'unknown')"
        exit 0
      fi

      sleep "$INTERVAL"
      ELAPSED=$((ELAPSED + INTERVAL))
      echo "  Waiting for boot_completed... (''${ELAPSED}s)"
    done

    echo "ERROR: Emulator did not finish booting within ''${BOOT_TIMEOUT}s" >&2
    echo "Last 20 lines of emulator log:" >&2
    tail -20 "''${TMPDIR:-/tmp}/emulator-$AVD_NAME.log" >&2
    kill "$EMU_PID" 2>/dev/null || true
    exit 1
  '';

in { inherit emulatorSdk emulatorComposition start-emulator; }
