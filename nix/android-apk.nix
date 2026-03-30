# nix/android-apk.nix
#
# Android APK build infrastructure for nix-key.
#
# The Android app is not a pure Nix derivation (Gradle needs network for deps),
# but the build environment (SDK, NDK, gomobile) is pinned via Nix for
# reproducibility. This module provides:
#
#   androidSdk        — Pinned Android SDK + NDK composition
#   gomobile          — gomobile tool configured with our SDK/NDK
#   build-android-apk — Script that builds debug/release APK
#
# Usage:
#   build-android-apk                       # debug APK (default)
#   build-android-apk --release             # release APK
#   build-android-apk --skip-aar            # skip gomobile AAR rebuild
#   build-android-apk --apk-only            # alias for --skip-aar
#
# Pinned versions (must match android/app/build.gradle.kts):
#   compileSdk   = 35
#   minSdk       = 29
#   buildTools   = 35.0.0
#   NDK          = 26.1.10909125 (r26b, for gomobile cross-compilation)
#   JDK          = 17
#   Gradle       = 8.11.1 (via wrapper in android/gradle/wrapper/)
#
{ pkgs, lib, }:

let
  # --- Pinned Android SDK/NDK versions ---
  androidComposition = pkgs.androidenv.composeAndroidPackages {
    platformVersions = [ "35" ];
    buildToolsVersions = [ "35.0.0" ];
    platformToolsVersion = "35.0.2";
    includeNDK = true;
    ndkVersions = [ "26.1.10909125" ];
    includeSources = false;
    includeSystemImages = false;
    includeEmulator = false;
    extraLicenses = [ "android-sdk-license" "android-sdk-preview-license" ];
  };

  androidSdk = androidComposition.androidsdk;
  androidHome = "${androidSdk}/libexec/android-sdk";

  # --- gomobile configured with our Android SDK/NDK ---
  gomobile = pkgs.gomobile.override {
    withAndroidPkgs = true;
    androidPkgs = androidComposition;
  };

  jdk = pkgs.jdk17;

  # --- Build script ---
  build-android-apk = pkgs.writeShellScriptBin "build-android-apk" ''
    set -euo pipefail

    # --- Parse arguments ---
    BUILD_TYPE="debug"
    SKIP_AAR=false
    for arg in "$@"; do
      case "$arg" in
        --release) BUILD_TYPE="release" ;;
        --skip-aar|--apk-only) SKIP_AAR=true ;;
        --help|-h)
          echo "Usage: build-android-apk [--release] [--skip-aar] [--help]"
          echo ""
          echo "Options:"
          echo "  --release    Build release APK (default: debug)"
          echo "  --skip-aar   Skip gomobile AAR rebuild"
          echo "  --apk-only   Alias for --skip-aar"
          exit 0
          ;;
        *)
          echo "Unknown argument: $arg" >&2
          exit 1
          ;;
      esac
    done

    # --- Environment ---
    export ANDROID_HOME="${androidHome}"
    export ANDROID_SDK_ROOT="${androidHome}"
    export JAVA_HOME="${jdk}"
    export PATH="${jdk}/bin:${pkgs.go}/bin:${gomobile}/bin:$PATH"

    REPO_ROOT="$(${pkgs.git}/bin/git rev-parse --show-toplevel 2>/dev/null || pwd)"
    ANDROID_DIR="$REPO_ROOT/android"

    echo "=== nix-key Android APK build ==="
    echo "ANDROID_HOME:  $ANDROID_HOME"
    echo "JAVA_HOME:     $JAVA_HOME"
    echo "Go:            $(go version)"
    echo "gomobile:      $(gomobile version 2>&1 || echo 'available')"
    echo "Build type:    $BUILD_TYPE"
    echo ""

    # --- Step 1: Build gomobile AAR from pkg/phoneserver ---
    if [ "$SKIP_AAR" = "false" ]; then
      echo "--- Step 1: Building gomobile AAR from pkg/phoneserver ---"

      # Ensure golang.org/x/mobile is in go.mod (required by gomobile bind)
      if ! ${pkgs.gnugrep}/bin/grep -q 'golang.org/x/mobile' "$REPO_ROOT/go.mod"; then
        echo "Adding golang.org/x/mobile dependency (required by gomobile bind)..."
        (cd "$REPO_ROOT" && go get golang.org/x/mobile/bind@latest)
      fi

      mkdir -p "$ANDROID_DIR/app/libs"

      echo "Running: gomobile bind -target android -androidapi 29 ./pkg/phoneserver"
      (
        cd "$REPO_ROOT"
        gomobile bind \
          -target android \
          -androidapi 29 \
          -o "$ANDROID_DIR/app/libs/phoneserver.aar" \
          ./pkg/phoneserver
      )

      echo "AAR built: $ANDROID_DIR/app/libs/phoneserver.aar"
      ls -lh "$ANDROID_DIR/app/libs/phoneserver.aar"
      echo ""
    else
      echo "--- Step 1: Skipping gomobile AAR build (--skip-aar) ---"
      if [ ! -f "$ANDROID_DIR/app/libs/phoneserver.aar" ]; then
        echo "ERROR: $ANDROID_DIR/app/libs/phoneserver.aar not found." >&2
        echo "Run without --skip-aar to build it first." >&2
        exit 1
      fi
      echo ""
    fi

    # --- Step 2: Build APK with Gradle ---
    echo "--- Step 2: Building $BUILD_TYPE APK with Gradle ---"
    cd "$ANDROID_DIR"

    GRADLE_TASK="assembleDebug"
    if [ "$BUILD_TYPE" = "release" ]; then
      GRADLE_TASK="assembleRelease"
    fi

    # Use the Gradle wrapper from the Android project
    if [ ! -x ./gradlew ]; then
      echo "ERROR: ./gradlew not found or not executable in $ANDROID_DIR" >&2
      exit 1
    fi

    ./gradlew "$GRADLE_TASK" \
      --no-daemon \
      -Dorg.gradle.java.home="${jdk}"

    # --- Output ---
    APK_DIR="$ANDROID_DIR/app/build/outputs/apk/$BUILD_TYPE"
    APK_PATH="$APK_DIR/app-''${BUILD_TYPE}.apk"

    # APK filename may vary; find it
    if [ ! -f "$APK_PATH" ]; then
      APK_PATH="$(find "$APK_DIR" -name '*.apk' -type f 2>/dev/null | head -1 || true)"
    fi

    if [ -z "$APK_PATH" ] || [ ! -f "$APK_PATH" ]; then
      echo "ERROR: APK not found in $APK_DIR" >&2
      exit 1
    fi

    echo ""
    echo "=== Build complete ==="
    echo "APK: $APK_PATH"
    echo "Size: $(du -h "$APK_PATH" | cut -f1)"
    echo ""
    echo "Install on emulator/device:"
    echo "  adb install $APK_PATH"
  '';

in { inherit androidSdk androidComposition gomobile build-android-apk; }
