FROM golang:1.23-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -mod=vendor \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /hopstat ./cmd/lg/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates iputils traceroute mtr
COPY --from=build /hopstat /usr/local/bin/hopstat
VOLUME ["/data"]
EXPOSE 8080 9090
ENTRYPOINT ["hopstat"]
CMD ["--mode=server", "--config=/data/config.yaml"]
