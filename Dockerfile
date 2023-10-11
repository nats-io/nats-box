#syntax=docker/dockerfile-upstream:1.6
FROM golang:1.21.3-alpine AS builder

LABEL maintainer "Derek Collison <derek@nats.io>"
LABEL maintainer "Waldemar Quevedo <wally@nats.io>"

ARG TARGETARCH

ARG VERSION_NATS
ARG VERSION_NATS_TOP
ARG VERSION_NSC
ARG VERSION_STAN

ENV GOPATH /go/${TARGETARCH}

RUN <<EOT 
    set -e
    mkdir -p ${GOPATH}

    go install -ldflags="-X main.version=${VERSION_NSC}" github.com/nats-io/nsc/v2@v${VERSION_NSC}
    go install github.com/nats-io/nats-top@v${VERSION_NATS_TOP}
    go install github.com/nats-io/natscli/nats@v${VERSION_NATS}
EOT

FROM alpine:3.18.4

ARG TARGETARCH

COPY --from=builder /go/${TARGETARCH}/bin/* /usr/local/bin

RUN <<EOT
    set -e
    apk -U upgrade
    apk add --no-cache ca-certificates curl figlet jq
    rm -rf /var/cache/apk && mkdir /var/cache/apk
    addgroup -g 1000 nats
    adduser -D -u 1000 -G nats nats
EOT

ENV NKEYS_PATH /nsc/nkeys
ENV XDG_DATA_HOME /nsc
ENV XDG_CONFIG_HOME /nsc/.config

COPY entrypoint.sh /entrypoint.sh

COPY profile.sh /etc/profile.d

RUN chmod +x /entrypoint.sh

WORKDIR /root

ENTRYPOINT ["/entrypoint.sh"]
