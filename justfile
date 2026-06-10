
binary := "faker"
config := "config.toml"

# Show available recipes
default:
    @just --list

# Run directly with go run (no build step needed)
run:
    go run . {{config}}

run-config config_path:
    go run . {{config_path}}

build:
    @echo "Building for current platform..."
    go build -o {{binary}} .
    @echo "Built: ./{{binary}}"

build-mac:
    @echo "Building for macOS (arm64)..."
    GOOS=darwin GOARCH=arm64 go build -o {{binary}}-mac-arm64 .
    @echo "Building for macOS (amd64)..."
    GOOS=darwin GOARCH=amd64 go build -o {{binary}}-mac-amd64 .
    @echo "Creating universal binary with lipo..."
    lipo -create -output {{binary}}-mac {{binary}}-mac-arm64 {{binary}}-mac-amd64 2>/dev/null \
        && rm {{binary}}-mac-arm64 {{binary}}-mac-amd64 \
        && echo "Built universal binary: ./{{binary}}-mac" \
        || (echo "lipo not available — keeping separate arm64 / amd64 binaries" && true)

build-windows:
    @echo "Building for Windows (amd64)..."
    GOOS=windows GOARCH=amd64 go build -o {{binary}}-windows-amd64.exe .
    @echo "Built: ./{{binary}}-windows-amd64.exe"

build-all: build-mac build-windows
    @echo "All platform builds complete."

clean-data:
    @echo "Removing fake_tracker_test/ ..."
    rm -rf fake_tracker_test/
    @echo "Done."

clean-bin:
    @echo "Removing binaries..."
    rm -f {{binary}} {{binary}}-mac {{binary}}-mac-arm64 {{binary}}-mac-amd64 {{binary}}-windows-amd64.exe
    @echo "Done."

clean: clean-data clean-bin
