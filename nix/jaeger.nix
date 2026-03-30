# Pre-built Jaeger v2 binary package.
# Jaeger was removed from nixpkgs; this fetches the official release binary.
{ lib, stdenv, fetchurl, autoPatchelfHook, }:
stdenv.mkDerivation rec {
  pname = "jaeger";
  version = "2.16.0";

  src = fetchurl {
    url =
      "https://github.com/jaegertracing/jaeger/releases/download/v${version}/jaeger-${version}-linux-amd64.tar.gz";
    sha256 = "13qa0aysnz2j9swsm3q13kiz44s6dpk19b4x1ci0cps0yxddyg15";
  };

  sourceRoot = "jaeger-${version}-linux-amd64";

  nativeBuildInputs = [ autoPatchelfHook ];

  installPhase = ''
    install -Dm755 jaeger $out/bin/jaeger
  '';

  meta = {
    description = "Distributed tracing platform (Jaeger v2 all-in-one)";
    homepage = "https://www.jaegertracing.io/";
    license = lib.licenses.asl20;
    platforms = [ "x86_64-linux" ];
    mainProgram = "jaeger";
  };
}
