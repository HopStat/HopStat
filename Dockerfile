FROM node:22-alpine AS frontend
WORKDIR /app/web/frontend
COPY web/frontend/package.json web/frontend/package-lock.json* ./
RUN npm install
COPY web/frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /lg ./cmd/lg

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -H appuser
COPY --from=builder /lg /usr/local/bin/lg
COPY --from=builder /app/web/dist /app/web/dist
COPY config.docker.yaml /app/config.yaml
RUN mkdir -p /app/data && chown appuser /app/data && chown appuser /app/web/dist
WORKDIR /app
USER appuser
EXPOSE 8011
ENTRYPOINT ["lg", "--mode=server", "--config=/app/config.yaml"]
