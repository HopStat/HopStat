# Build frontend with Node.js 26.12.1, the October 21, 2026 security release.
# Docker Hub publishes node:26.12.1 as an empty index until each architecture
# finishes building, so the official tarball is installed instead.
FROM debian:bookworm-slim AS frontend
ARG TARGETARCH
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl libatomic1 xz-utils \
 && if [ -z "$TARGETARCH" ]; then \
      case "$(uname -m)" in \
        aarch64|arm64) TARGETARCH=arm64 ;; \
        x86_64) TARGETARCH=amd64 ;; \
        *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;; \
      esac; \
    fi \
 && case "$TARGETARCH" in \
      arm64) arch=arm64; sha=42aa04922591ee5d4dbc7740d452621acf28508239daa512f5ee8c47ee224fdb ;; \
      amd64) arch=x64; sha=6170b2d474e1c626f43cff3adc118df9fe3b573258a4fc3d850c8dbc6b079bbd ;; \
      *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac \
 && curl -fsSL "https://nodejs.org/dist/v26.12.1/node-v26.12.1-linux-${arch}.tar.xz" -o /tmp/node.tar.xz \
 && echo "${sha}  /tmp/node.tar.xz" | sha256sum -c - \
 && tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 \
 && rm /tmp/node.tar.xz \
 && apt-get purge -y curl xz-utils \
 && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/* \
 && node -v | grep -qx 'v26.12.1'
WORKDIR /src
COPY web/frontend/package.json web/frontend/package-lock.json ./
RUN npm ci
COPY web/frontend/ ./
RUN npm run build
# outDir: ../dist → writes to /dist

# Build Go binary with embedded frontend
FROM golang:1.25-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY vendor/ vendor/
COPY go.mod go.sum ./
COPY web/ web/
COPY --from=frontend /dist web/dist/
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -mod=vendor \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /hopstat ./cmd/lg/

# Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates iputils traceroute mtr
COPY --from=build /hopstat /usr/local/bin/hopstat
VOLUME ["/data"]
WORKDIR /data
EXPOSE 8080 9090
ENTRYPOINT ["hopstat"]
CMD ["--mode=server", "--config=/data/config.yaml"]
