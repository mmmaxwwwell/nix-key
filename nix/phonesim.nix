{
  lib,
  buildGoModule,
  go,
}:
buildGoModule {
  pname = "phonesim";
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
      ../test/phonesim
    ];
  };

  vendorHash = "sha256-mUm/bTgGwoyCFxUlYn4sFCU8OGcpOeVGoaJWkb5PHvI=";

  # Pin Go version from nixpkgs (matches flake devShell)
  inherit go;

  subPackages = [ "test/phonesim" ];

  ldflags = [
    "-s"
    "-w"
  ];

  meta = {
    description = "Phone simulator for nix-key E2E testing";
    mainProgram = "phonesim";
    license = lib.licenses.mit;
  };
}
