{
  description = "nix-key — SSH agent that delegates signing to a paired phone";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    {
      # System-independent outputs
      nixosModules.default = import ./nix/module.nix;

      overlays.default = final: _prev: {
        nix-key = final.callPackage ./nix/package.nix { };
        phonesim = final.callPackage ./nix/phonesim.nix { };
      };
    }
    // flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ self.overlays.default ];
          config.allowUnfree = true;
          config.android_sdk.accept_license = true;
        };

        # Android build infrastructure (SDK, NDK, gomobile, build script)
        androidApk = import ./nix/android-apk.nix {
          inherit pkgs;
          lib = pkgs.lib;
        };

        # Android emulator infrastructure (E2E testing)
        androidEmulator = import ./nix/android-emulator.nix {
          inherit pkgs;
          lib = pkgs.lib;
        };
      in
      {
        packages.default = pkgs.nix-key;
        packages.phonesim = pkgs.phonesim;
        packages.build-android-apk = androidApk.build-android-apk;
        packages.android-sdk = androidApk.androidSdk;
        packages.start-emulator = androidEmulator.start-emulator;
        packages.emulator-sdk = androidEmulator.emulatorSdk;

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            # Go toolchain
            go
            gopls
            gotools

            # Protobuf
            protobuf
            protoc-gen-go
            protoc-gen-go-grpc

            # Security / encryption
            age

            # Tailscale / Headscale
            headscale
            tailscale

            # Linting and formatting
            golangci-lint
            nixfmt-rfc-style

            # Secret scanning
            gitleaks

            # Android build tools
            androidApk.gomobile
            androidApk.build-android-apk

            # Android emulator (E2E testing)
            androidEmulator.start-emulator
          ];

          shellHook = ''
            # Install gitleaks pre-commit hook
            git config --local core.hooksPath .githooks

            echo "nix-key dev shell ready"
            echo "  Go:             $(go version)"
            echo "  protoc:         $(protoc --version)"
            echo "  age:            $(age --version)"
            echo "  golangci-lint:  $(golangci-lint --version 2>&1 | head -1)"
          '';
        };
      }
      // nixpkgs.lib.optionalAttrs (system == "x86_64-linux") {
        # NixOS VM tests (added by T043+)
        checks =
          let
            # Helper to import a NixOS VM test with the nix-key module pre-loaded
            callTest =
              testPath:
              nixpkgs.legacyPackages.${system}.nixosTest (
                import testPath {
                  inherit pkgs;
                  nixKeyModule = self.nixosModules.default;
                }
              );
            testDir = ./nix/tests;
            hasTests = builtins.pathExists testDir;
          in
          nixpkgs.lib.optionalAttrs hasTests (
            nixpkgs.lib.optionalAttrs (builtins.pathExists (testDir + "/service-test.nix")) {
              service-test = callTest (testDir + "/service-test.nix");
            }
            // nixpkgs.lib.optionalAttrs (builtins.pathExists (testDir + "/pairing-test.nix")) {
              pairing-test = callTest (testDir + "/pairing-test.nix");
            }
            // nixpkgs.lib.optionalAttrs (builtins.pathExists (testDir + "/signing-test.nix")) {
              signing-test = callTest (testDir + "/signing-test.nix");
            }
            // nixpkgs.lib.optionalAttrs (builtins.pathExists (testDir + "/jaeger-test.nix")) {
              jaeger-test = callTest (testDir + "/jaeger-test.nix");
            }
            // nixpkgs.lib.optionalAttrs (builtins.pathExists (testDir + "/tracing-e2e-test.nix")) {
              tracing-e2e-test = callTest (testDir + "/tracing-e2e-test.nix");
            }
          );
      }
    );
}
