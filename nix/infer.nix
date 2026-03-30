# Pre-built Facebook Infer static analyzer.
# Used for RacerD race condition detection in Android/Kotlin code.
{
  lib,
  stdenv,
  fetchurl,
  autoPatchelfHook,
  gmp,
  mpfr,
  sqlite,
  zlib,
}:
stdenv.mkDerivation rec {
  pname = "infer";
  version = "1.2.0";

  src = fetchurl {
    url = "https://github.com/facebook/infer/releases/download/v${version}/infer-linux-x86_64-v${version}.tar.xz";
    sha256 = "21504063fb3a1dbc7919f34dc6e50ca0d35f50b996d91deb7b8bea8243d52d82";
  };

  sourceRoot = "infer-linux-x86_64-v${version}";

  nativeBuildInputs = [ autoPatchelfHook ];

  buildInputs = [
    gmp
    mpfr
    sqlite
    zlib
    stdenv.cc.cc.lib
  ];

  installPhase = ''
    mkdir -p $out
    cp -r bin lib share $out/
  '';

  meta = {
    description = "Facebook Infer static analyzer for Java/C/C++/ObjC (includes RacerD)";
    homepage = "https://fbinfer.com/";
    license = lib.licenses.mit;
    platforms = [ "x86_64-linux" ];
    mainProgram = "infer";
  };
}
