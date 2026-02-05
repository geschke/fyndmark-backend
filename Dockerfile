# ---------- Build stage ----------
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

# Build the binary (CGO off is fine; Debian runtime doesn't care)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/fyndmark .


# ---------- Hugo stage (pinned extended) ----------
FROM debian:bookworm-slim AS hugo

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
  && rm -rf /var/lib/apt/lists/*

ARG TARGETARCH
ARG HUGO_VERSION=0.155.2

RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) HUGO_ARCH="Linux-64bit" ;; \
      arm64) HUGO_ARCH="Linux-ARM64" ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/hugo.tar.gz \
      "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_${HUGO_ARCH}.tar.gz"; \
    tar -xzf /tmp/hugo.tar.gz -C /tmp; \
    mv /tmp/hugo /usr/local/bin/hugo; \
    chmod +x /usr/local/bin/hugo; \
    /usr/local/bin/hugo version


# ---------- Runtime stage ----------
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    openssh-client \
    tzdata \
  && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd --system fyndmark && useradd --system --gid fyndmark --create-home --home-dir /home/fyndmark fyndmark

WORKDIR /app

# Copy binaries
COPY --from=builder /bin/fyndmark /bin/fyndmark
COPY --from=hugo /usr/local/bin/hugo /usr/local/bin/hugo

# Default config + working dirs
RUN mkdir -p /config /app/website \
 && chown -R fyndmark:fyndmark /config /app/website

VOLUME ["/config"]
VOLUME ["/app/website"]

USER fyndmark

EXPOSE 8080

ENTRYPOINT ["/bin/fyndmark"]
CMD ["serve"]
