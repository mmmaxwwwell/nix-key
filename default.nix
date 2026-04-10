# Non-flake entry point for nix-key.
# Usage: import ./path/to/nix-key { inherit pkgs; }
# Returns: { package, module, overlay }
{
  pkgs ? import <nixpkgs> { },
}:
{
  package = pkgs.callPackage ./nix/package.nix { };
  module = import ./nix/module.nix;
  overlay = final: _prev: {
    nix-key = final.callPackage ./nix/package.nix { };
  };
}
