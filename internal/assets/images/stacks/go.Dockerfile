# Go {{.Version}}. Modules and the build cache persist in ~/.cache, go install puts binaries in
# ~/.local/bin.
RUN {{mounts "stacks/go.sh"}}VERSION={{.Version}} sh /tmp/jw/go.sh
ENV PATH=/usr/local/go/bin:$PATH \
    GOPATH=${SANDBOX_HOME}/.local/share/go \
    GOMODCACHE=${SANDBOX_HOME}/.cache/go/mod \
    GOBIN=${SANDBOX_HOME}/.local/bin
