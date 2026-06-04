{ pkgs ? import <nixpkgs> {} }:

with pkgs;

mkShell {
  buildInputs = [
    go
    gopls
    gotools
    go-tools
    gcc
  ] ++ lib.optionals stdenv.isLinux [
    xorg.libX11.dev
  ] ++ lib.optionals stdenv.isDarwin [
    darwin.apple_sdk.frameworks.Cocoa
  ];

  shellHook = ''
    export CGO_ENABLED=1
  '';
}
