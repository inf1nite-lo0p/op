# op — fuzzy-pick a git project to cd into.
#
# Run `just` (no args) to see the recipe list.

# Where the binary lands. Override with `INSTALL_DIR=... just install`.
install_dir := env_var_or_default("INSTALL_DIR", env_var("HOME") + "/.local/bin")
shell_dir   := env_var_or_default("SHELL_DIR",   env_var("HOME") + "/.local/share/op")

# Default recipe: list available recipes.
default:
    @just --list

# Build the picker binary into ./bin/op-bin.
build:
    mkdir -p bin
    go build -o bin/op-bin ./cmd/op-bin

# Run all tests.
test:
    go test ./...

# Tests + vet + fmt check. CI runs the same.
check:
    go vet ./...
    test -z "$(gofmt -l .)"
    go test ./...

# Install op-bin into $INSTALL_DIR and the shell shim into $SHELL_DIR.
# Add `source $SHELL_DIR/op.bash` to your shell rc afterwards.
install: build
    install -d "{{install_dir}}"
    install -m 0755 bin/op-bin "{{install_dir}}/op-bin"
    install -d "{{shell_dir}}"
    install -m 0644 shell/op.bash "{{shell_dir}}/op.bash"
    @echo
    @echo "Installed:"
    @echo "  binary: {{install_dir}}/op-bin"
    @echo "  shim:   {{shell_dir}}/op.bash"
    @echo
    @echo "Add this to your ~/.bashrc (or ~/.zshrc):"
    @echo "  source {{shell_dir}}/op.bash"

# Remove the installed binary and shim.
uninstall:
    rm -f "{{install_dir}}/op-bin"
    rm -f "{{shell_dir}}/op.bash"
    rmdir "{{shell_dir}}" 2>/dev/null || true

# Tidy module deps.
tidy:
    go mod tidy

# Clean build artefacts.
clean:
    rm -rf bin
