FROM golang:1.25.5 AS builder

ARG VERSION=dev

WORKDIR /src
COPY . .

RUN mkdir -p /build && \
    targets=" \
      linux/amd64 \
      linux/arm64 \
      linux/386 \
      darwin/amd64 \
      darwin/arm64 \
      windows/amd64 \
      windows/arm64 \
      windows/386 \
    " && \
    for target in $targets; do \
      os="${target%/*}"; \
      arch="${target#*/}"; \
      ext=""; \
      if [ "$os" = "windows" ]; then ext=".exe"; fi; \
      output="/build/code-agents-${os}-${arch}${ext}"; \
      echo "Building ${output} ..."; \
      CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
        go build -ldflags "-s -w -X main.version=${VERSION}" \
        -o "$output" ./cmd/code-agents; \
    done
