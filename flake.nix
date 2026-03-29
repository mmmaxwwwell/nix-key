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
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
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
          ];

          shellHook = ''
            echo "nix-key dev shell ready"
            echo "  Go:             $(go version)"
            echo "  protoc:         $(protoc --version)"
            echo "  age:            $(age --version)"
            echo "  golangci-lint:  $(golangci-lint --version 2>&1 | head -1)"
          '';
        };
      }
    );
}
