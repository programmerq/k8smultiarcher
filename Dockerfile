# Multi-stage Dockerfile for building a static Go binary and producing a minimal runtime image.
ARG GO_VERSION=1.20
FROM golang:${GO_VERSION}-alpine AS builder

# Install git (for 'go get' if needed) and ca-certificates
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Copy go module files first for efficient caching (if present)
COPY go.mod go.sum ./
RUN if [ -f go.mod ]; then go mod download; fi

# Copy the rest of the source
COPY . .

# Build the binary. Adjust the package path or output name if your repo layout differs.
# Use CGO_ENABLED=0 to produce a statically-linked binary for scratch.
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w" -o /out/k8smultiarcher ./...

# Final minimal image
FROM scratch AS final
# Add CA certificates for any TLS needs
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/k8smultiarcher /k8smultiarcher

EXPOSE 8080

ENTRYPOINT ["/k8smultiarcher"]
