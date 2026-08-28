{
  description = "A simple Go package";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  };

  outputs = { self, nixpkgs }:
    let
      version = "0.2.0-yarn";
      pname = "tsui";

      supportedSystems = [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      
      # Define this out here so it doesn't cause recursive evaluation loops
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
          
          linuxInterpreters = {
            x86_64 = "/lib64/ld-linux-x86-64.so.2";
            aarch64 = "/lib/ld-linux-aarch64.so.1";
          };
          linuxInterpreter = linuxInterpreters.${pkgs.stdenv.hostPlatform.parsed.cpu.name} or "/lib64/ld-linux-x86-64.so.2";
        in
        {
          # FIXED: Named explicitly 'tsui' so your external scripts/commands don't break
          tsui = pkgs.buildGoModule {
            inherit pname version;
            src = ./.;

            preBuild = if pkgs.stdenv.hostPlatform.isLinux && pkgs.stdenv.hostPlatform.isx86_64 then ''
              export GODEBUG=asyncpreemptoff=1
            '' else null;

            ldflags = [ "-X main.Version=${version}" ];

            vendorHash = "sha256-FIbkPE5KQ4w7Tc7kISQ7ZYFZAoMNGiVlFWzt8BPCf+A=";

            nativeBuildInputs = if pkgs.stdenv.hostPlatform.isLinux then [ pkgs.pkg-config ] else [ ];

            buildInputs = 
              if pkgs.stdenv.hostPlatform.isLinux then [ pkgs.libx11 ]
              else if pkgs.stdenv.hostPlatform.isDarwin then [ pkgs.darwin.apple_sdk.frameworks.Cocoa ]
              else [ ];
          };

          # Bind 'default' to 'tsui' so 'nix build' works out of the box
          default = self.packages.${system}.tsui;

          tsui_no_nix_ld = self.packages.${system}.tsui.overrideAttrs (oldAttrs: {
            preFixup = if pkgs.stdenv.hostPlatform.isLinux then ''
              patchelf --remove-rpath --set-interpreter ${linuxInterpreter} $out/bin/${pname}
            '' else null;
          });
        });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [ go gopls gotools go-tools ];

            nativeBuildInputs = if pkgs.stdenv.hostPlatform.isLinux then [ pkgs.pkg-config ] else [ ];
            buildInputs = 
              if pkgs.stdenv.hostPlatform.isLinux then [ pkgs.libx11 ]
              else if pkgs.stdenv.hostPlatform.isDarwin then [ pkgs.darwin.apple_sdk.frameworks.Cocoa ]
              else [ ];
          };
        });
    };
}
