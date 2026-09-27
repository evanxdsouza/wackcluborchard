# syntax=docker/dockerfile:1

# 1. dashboard: plain tsc, no bundler
FROM node:22-alpine AS web
RUN npm install -g typescript@5 && apk add --no-cache bash coreutils
WORKDIR /src/web
COPY web/ ./
RUN ./build.sh

# 2. binaries: Go standard library only
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/wackcluborchard-server ./cmd/wackcluborchard-server \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/wackcluborchard ./cmd/wackcluborchard \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/wackcluborchardctl ./cmd/wackcluborchardctl

# 3. runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 wackcluborchard && mkdir /data && chown wackcluborchard /data
COPY --from=build /out/ /usr/local/bin/
USER wackcluborchard
ENV WACKCLUBORCHARD_DATA=/data PORT=8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["wackcluborchard-server"]
