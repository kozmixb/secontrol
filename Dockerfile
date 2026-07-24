FROM golang:1.24-alpine AS build
WORKDIR /build
RUN apk add --no-cache ca-certificates
COPY src/go.mod src/go.sum ./
RUN go mod download all
COPY src/ .
RUN asset_version="$(sha256sum assets/app.js assets/styles.css | sha256sum | cut -c1-12)" \
    && sed -i "s/__ASSET_VERSION__/${asset_version}/g" assets/index.html
RUN go test ./... \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/secontrol ./cmd/secontrol

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S secontrol && adduser -S -G secontrol secontrol
WORKDIR /app
COPY --from=build /out/secontrol /usr/local/bin/secontrol
RUN mkdir -p /data && chown secontrol:secontrol /data
USER secontrol
EXPOSE 5000
VOLUME ["/data"]
ENV APP_ADDR=:5000 APP_DATA_DIR=/data
ENTRYPOINT ["secontrol"]
