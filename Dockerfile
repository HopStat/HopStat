# Build frontend
FROM node:24-alpine AS frontend
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
