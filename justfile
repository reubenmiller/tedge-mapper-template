
[private]
default:
    @just --list -f "{{justfile()}}"

# Run locally
start *ARGS='':
    go run main.go serve --verbose {{ARGS}}

# Run tests
test *ARGS='':
    go test ./... {{ARGS}}

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

release-snapshot:
    just -f "{{justfile()}}" release --snapshot

# Run local container (requires ko)
run-container-ko:
    docker run --rm -v $(PWD)/routes:/routes -p 8080:8080 $(ko build . --local) serve --host "host.docker.internal:1883" --dir /routes --verbose

# Build container (requires ko)
build-container-ko:
    KO_DOCKER_REPO=ghcr.io/reubenmiller/tedge-mapper-template ko build . --push=false --tags latest
