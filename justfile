# op — fuzzy-pick a git project to cd into.
#
# Run `just` (no args) to see the recipe list.

# Where the binary lands. Override with `INSTALL_DIR=... just install`.
install_dir := env_var_or_default("INSTALL_DIR", env_var("HOME") + "/.local/bin")

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

# Build from source and install op-bin into $INSTALL_DIR.
# After install, wire up the shell shim by adding this to your rc:
#   eval "$(op-bin shell-init bash)"   # or zsh
install: build
    install -d "{{install_dir}}"
    install -m 0755 bin/op-bin "{{install_dir}}/op-bin"
    @echo
    @echo "Installed: {{install_dir}}/op-bin"
    @echo
    @echo "Add this to your ~/.bashrc (or ~/.zshrc):"
    @echo '  eval "$(op-bin shell-init bash)"'

# Remove the installed binary.
uninstall:
    rm -f "{{install_dir}}/op-bin"

# Render the README demo GIF from assets/demo.tape.
# Requires vhs, ttyd, and ffmpeg on PATH.
demo: build
    PATH="$PWD/bin:$PATH" vhs assets/demo.tape

# Tidy module deps.
tidy:
    go mod tidy

# Clean build artefacts.
clean:
    rm -rf bin
