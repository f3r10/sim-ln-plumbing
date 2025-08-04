
{
  description = "A reproducible Go development environment and build using Nix flakes.";

  inputs = {
    # Nixpkgs provides all the packages (Go compiler, tools, etc.)
    # We're using the 'nixos-unstable' branch for more up-to-date packages.
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

    # flake-utils simplifies defining outputs for multiple systems (x86_64-linux, aarch64-linux, etc.)
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    # This helper iterates over common systems (e.g., x86_64-linux, aarch64-linux)
    # and applies the function to each, creating outputs for those systems.
    flake-utils.lib.eachDefaultSystem (system:
      let
        # Import nixpkgs for the current system.
        pkgs = import nixpkgs { inherit system; };

        # Define the Go version to use. You can change this to "go_1_21", "go_1_20", etc.
        # Ensure the specified version is available in your chosen nixpkgs branch.
        goVersion = "go_1_24"; # Using Go 1.22

        # Define your Go application as a Nix package.
        # This uses pkgs.buildGoModule, which is specifically designed for Go projects.
        myGoApp = pkgs.buildGoModule {
          pname = "sim-ln-plumbing"; # The name of your package
          version = "0.1.0";   # The version of your package

          # The source of your Go project.
          # `./.` means the current directory where flake.nix resides.
          src = ./.;

          # The Go module name, which should match the module name in your go.mod file.
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";
          # IMPORTANT: Replace the placeholder above with the actual hash of your Go modules.
          # You'll get an error with the correct hash when you first run `nix build` or `nix develop`.

          # Optional: Add any specific build flags for your Go application.
          # buildFlags = [ "-ldflags=-s -w" ]; # Example: strip symbols and DWARF debug info
        };

      in
      {
        # Define the default package that can be built by `nix build`.
        # This will build your Go application.
        packages.default = myGoApp;

        # Define the default development shell that can be entered with `nix develop`.
        devShells.default = pkgs.mkShell {
          # Packages available in your development shell.
          packages = with pkgs; [
            # The Go compiler
            pkgs.${goVersion}

            # Go language server (highly recommended for IDEs like VS Code, Neovim)
            gopls

            # Other useful Go tools (e.g., goimports, golint, etc.)
            go-tools

            # Git is often needed for Go module fetching
            git

	    # LSP
	    gopls
          ];

          # Commands to run when entering the shell.
          shellHook = ''
            echo "Welcome to the reproducible Go development shell!"
            echo "Your Go version: $(go version)"
            echo "You can build your app with 'go build -o my-go-app-dev'."
            echo "To build the Nix package, run 'nix build'."
            echo ""
            # Add a local 'bin' directory to PATH for compiled binaries
            export PATH="$PWD/bin:$PATH"
          '';
        };
      });
}
