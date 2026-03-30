{ lib, buildGoModule, go, }:
buildGoModule {
  pname = "nix-key";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../gen
      ../internal
      ../pkg
    ];
  };

  vendorHash = "sha256-z2s5/D326uD4MnTFRJwDSgwB4UYON0Toez0ZyVaagjU=";

  # Pin Go version from nixpkgs (matches flake devShell)
  inherit go;

  subPackages = [ "cmd/nix-key" ];

  ldflags = [ "-s" "-w" ];

  meta = {
    description =
      "SSH agent that delegates signing to a paired Android phone over Tailscale with mTLS";
    mainProgram = "nix-key";
    license = lib.licenses.mit;
  };
}
