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

  vendorHash = "sha256-qv6Yu7Tf3xWy1ksQjsyLhN+uRgdcNwUY2ZZOSKm3bZY=";

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
