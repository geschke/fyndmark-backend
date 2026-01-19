# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

# Install build tools (if needed later)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy go.mod / go.sum first to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the binary
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/fyndmark .

# ---------- Runtime stage ----------
FROM alpine:3.23

# Add CA certificates for HTTPS (Turnstile, SMTP with TLS, etc.)
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -S fyndmark && adduser -S fyndmark -G fyndmark

WORKDIR /app

# Copy binary from builder
COPY --from=builder /bin/fyndmark /bin/fyndmark

# Optional: directory for config files (volume mount or baked-in config)
# You already search ".", "./config" and "/config" in your code,
# so we provide /config as a reasonable default.
RUN mkdir -p /config
VOLUME ["/config"]

# Switch to non-root user
USER fyndmark

# Expose default HTTP port used by fyndmark (server.listen)
EXPOSE 8080

# Default command
# You can override config path and flags via environment or args:
#   docker run ... fyndmark --config /config/config.yaml
ENTRYPOINT ["/bin/fyndmark"]
CMD ["serve"]
