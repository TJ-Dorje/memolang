# Nix flake for memolang, consumed by github.com/TJ-Dorje/dm-homelab-nixos.
# That repo runs the binary as systemd services (web: `memolang`, worker:
# `memolang worker`) with WorkingDirectory=$out/share/memolang, because the
# binary reads ./templates and ./static relative to its working directory.
{
  description = "memolang: server-rendered Go flashcard app";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      lib = nixpkgs.lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
        default = (pkgs.buildGoModule.override { go = pkgs.go_1_27; }) {
          pname = "memolang";
          version = self.shortRev or self.dirtyShortRev or "dev";

          # Only what the build and runtime need; keeps the local binary,
          # sqlite files, test/, testdata/ and docs/ out of the store.
          src = lib.fileset.toSource {
            root = ./.;
            fileset = lib.fileset.unions [
              ./go.mod
              ./go.sum
              ./main.go
              ./internal
              ./templates
              ./static
            ];
          };

          vendorHash = "sha256-sjL5Lan+7AeihfRaFygevgxiy8Rsdg+4hcP0jFLhCjk=";
          subPackages = [ "." ];
          env.CGO_ENABLED = 0;
          ldflags = [
            "-s"
            "-w"
          ];

          # Tests need Postgres/NATS; they run in the repo's own CI.
          doCheck = false;

          postInstall = ''
            mkdir -p $out/share/memolang
            cp -r templates static $out/share/memolang/
          '';

          meta.mainProgram = "memolang";
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt-tree);
    };
}
