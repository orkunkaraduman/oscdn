# syntax=docker.io/docker/dockerfile:1.4.3

FROM golang:1.20.7-alpine3.18 AS builder

WORKDIR /src
COPY . .

RUN mkdir /target

RUN \
    --mount=type=cache,target=/go \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go install -v . && cp -a /go/bin/oscdn /target/

FROM alpine:3.18

RUN apk upgrade --no-cache && apk add --no-cache \
    bash openssl ca-certificates tzdata

WORKDIR /app

COPY --from=builder /target/oscdn .
RUN touch oscdn.conf
COPY --from=builder /src/config.yaml .
RUN mkdir /store

ENV OSCDN_CONF=oscdn.conf

ENTRYPOINT [ "./oscdn", "-store-path", "/store" ]
