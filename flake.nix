{
  description = "Monocle Operator Project";
  nixConfig.bash-prompt = "[nix(monocle-operator)] ";
  inputs = { nixpkgs.url = "github:nixos/nixpkgs/23.05"; };

  outputs = { self, nixpkgs }:
    let pkgs = nixpkgs.legacyPackages.x86_64-linux.pkgs;
    in {
      devShells.x86_64-linux.default = pkgs.mkShell {
        name = "Monocle-Operator build environment";
        buildInputs = [
          # 1.27.1 in nixpkgs
          pkgs.kubectl
          # 1.20.4 in nixpkgs
          pkgs.go
          # 0.11.0 in nixpkgs
          pkgs.gopls
          pkgs.k9s
        ];
        shellHook = ''
          echo "Welcome in $name"
        '';
      };
    };
}
