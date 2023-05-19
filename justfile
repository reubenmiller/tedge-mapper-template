
# Build snapshot
build *ARGS='':
    goreleaser build --clean --snapshot {{ARGS}}

# Release artifacts
release *ARGS='':
    mkdir -p output
    go run main.go completion bash > output/completions.bash
    go run main.go completion zsh > output/completions.zsh
    go run main.go completion fish > output/completions.fish

    docker context use default
    goreleaser release --clean --snapshot {{ARGS}}

# Run app in local container
run-container:
    docker run --rm -v $(PWD)/routes:/routes -p 8080:8080 $(ko build . --local) serve --host "host.docker.internal:1883" --dir /routes --verbose

# Build container
build-container:
    KO_DOCKER_REPO=ghcr.io/reubenmiller/tedge-mapper-template ko build . --push=false --tags latest
