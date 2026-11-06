FROM golang:1.27.3-trixie

ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=$USER_UID

RUN apt-get update \
    && apt-get -y install --no-install-recommends git openssh-client less sudo \
    && rm -rf /var/lib/apt/lists/* \
    && GOPATH=/tmp/gotools GOBIN=/usr/local/bin go install golang.org/x/tools/gopls@v0.24.0 \
    && GOPATH=/tmp/gotools GOBIN=/usr/local/bin go install github.com/go-delve/delve/cmd/dlv@v1.27.2 \
    && GOPATH=/tmp/gotools go clean -cache -modcache \
    && rm -rf /tmp/gotools \
    && groupadd --gid "$USER_GID" "$USERNAME" \
    && useradd -s /bin/bash --uid "$USER_UID" --gid "$USER_GID" -m "$USERNAME" \
    && echo "$USERNAME ALL=(root) NOPASSWD:ALL" > "/etc/sudoers.d/$USERNAME" \
    && chmod 0440 "/etc/sudoers.d/$USERNAME"
