
build *ARGS='':
    goreleaser build --rm-dist --snapshot {{ARGS}}
