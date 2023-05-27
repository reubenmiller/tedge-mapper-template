
[private]
default:
    @just --list -f "{{justfile()}}"

# Start the demo with the latest built package
demo arch='arm64' args='--no-prompt':
    just release-local
    rm -f demo/dist/tedge-mapper-template*.deb
    cp dist/tedge-mapper-template_*{{arch}}*deb demo/dist/
    cd demo && just up
    cd demo && just bootstrap {{args}}

# Open a shell on the demo (if running)
demo-shell:
    cd demo && just shell-main

# Install dev dependencies
setup-dev:
    go install github.com/reubenmiller/commander/v3/cmd/commander@v3.0.2

# Run locally
start *ARGS='':
    go run main.go serve {{ARGS}}

# Run tests
test *ARGS='':
    go test ./... {{ARGS}}

# Test routes
test-routes *ARGS='': setup-dev
    commander test --config ./tests/config.yaml {{ARGS}} --dir tests/

# Build for current target
build *ARGS='':
    goreleaser build --clean --snapshot --single-target {{ARGS}}

# Release all artifacts
release *ARGS='':
    mkdir -p output
    go run main.go completion bash > output/completions.bash
    go run main.go completion zsh > output/completions.zsh
    go run main.go completion fish > output/completions.fish

    docker context use default
    goreleaser release --clean --auto-snapshot {{ARGS}}

release-local:
    just -f "{{justfile()}}" release --snapshot

# Run local container (requires ko)
run-container-ko:
    docker run --rm -v $(PWD)/routes:/routes -p 8080:8080 $(ko build . --local) --host "host.docker.internal:1883" --dir /routes

# Build container (requires ko)
build-container-ko:
    KO_DOCKER_REPO=ghcr.io/reubenmiller/tedge-mapper-template ko build . --push=false --tags latest
