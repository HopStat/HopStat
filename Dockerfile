# ── Frontend build ────────────────────────────────────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /src/web/frontend
COPY web/frontend/package.json web/frontend/package-lock.json ./
RUN npm ci
COPY web/frontend/ ./
RUN npm run build
# outDir '../dist' → output lands at /src/web/dist/

# ── Go build ──────────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /src/web/dist ./web/dist

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /hopstat ./cmd/lg/

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.20

# Network tools required for ping / traceroute / mtr
RUN apk add --no-cache ca-certificates iputils traceroute mtr

COPY --from=build /hopstat /usr/local/bin/hopstat

# /data holds config, database, GeoIP databases, and logo uploads.
# Mount a named volume here to persist state across container restarts.
VOLUME ["/data"]

EXPOSE 8080 9090

ENTRYPOINT ["hopstat"]
CMD ["--mode=server", "--config=/data/config.yaml"]
