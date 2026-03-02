{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs?ref=nixos-unstable";

  outputs = {
    self,
    nixpkgs,
    ...
  }: let
    systems = ["x86_64-linux" "aarch64-linux"];
    forEachSystem = nixpkgs.lib.genAttrs systems;
    pkgsForEach = nixpkgs.legacyPackages;
  in {
    packages = forEachSystem (system: {
      default = pkgsForEach.${system}.callPackage ./nix/package.nix {};
    });

    devShells = forEachSystem (system: {
      default = pkgsForEach.${system}.callPackage ./nix/shell.nix {};
    });

    formatter = forEachSystem (system: let
      pkgs = pkgsForEach.${system};
    in
      pkgs.writeShellApplication {
        name = "nix3-fmt-wrapper";
        runtimeInputs = [
          pkgs.alejandra
          pkgs.fd
          pkgs.prettier
          pkgs.deno
          pkgs.go # provides gofmt
          pkgs.golines
        ];

        text = ''
          # Format Nix files with Alejandra
          fd "$@" -t f -e nix -x alejandra -q '{}'

          # Format HTML & Javascript files with Prettier
          fd "$@" -t f -e html -e js -x prettier -w '{}'

          # Format Markdown with Deno's Markdown formatter
          fd "$@" -t f -e md -x deno fmt -q '{}'

          # Format go files with both gofmt & golines
          fd "$@" -t f -e go -x golines -l -w --max-len=110 \
            --base-formatter=gofmt \
            --shorten-comments \
            --ignored-dirs=.direnv '{}'
        '';
      });

    nixosModules = {
      watchdog = import ./nix/module.nix self;
      default = self.nixosModules.watchdog;
    };

    hydraJobs = self.packages;
  };
}
