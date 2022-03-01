FROM golang:1.17-alpine AS builder

LABEL maintainer "Derek Collison <derek@nats.io>"
LABEL maintainer "Waldemar Quevedo <wally@nats.io>"
LABEL maintainer "Jaime Piña <jaime@nats.io>"

WORKDIR $GOPATH/src/github.com/nats-io/

RUN apk add -U --no-cache git binutils

RUN go install github.com/nats-io/nats-top@latest

RUN go install -ldflags "-X main.version=2.6.0" github.com/nats-io/nsc@2.6.0

RUN mkdir -p src/github.com/nats-io && \
    cd src/github.com/nats-io/ && \
    git clone https://github.com/nats-io/natscli.git && \
    cd natscli/nats && \
    git fetch origin && \
    git checkout v0.0.28 && \
    go build -ldflags "-s -w -X main.version=0.0.28" -o /nats

RUN go install github.com/nats-io/stan.go/examples/stan-pub@latest
RUN go install github.com/nats-io/stan.go/examples/stan-sub@latest
RUN go install github.com/nats-io/stan.go/examples/stan-bench@latest

# Simple tools
COPY . .
RUN go install
RUN strip /go/bin/*

FROM alpine:3.15

RUN apk add -U --no-cache ca-certificates figlet

COPY --from=builder /go/bin/* /usr/local/bin/
COPY --from=builder /nats /usr/local/bin/

RUN cd /usr/local/bin/ && \
    ln -s nats-box nats-pub && \
    ln -s nats-box nats-sub && \
    ln -s nats-box nats-req && \
    ln -s nats-box nats-rply

WORKDIR /home/nats
RUN addgroup -S -g 1002 natsgroup && adduser -S -u 1001 nats -G natsgroup
USER nats

ENV NKEYS_PATH /home/nats/nsc/nkeys
ENV XDG_DATA_HOME /home/nats/nats/nsc
ENV XDG_CONFIG_HOME /home/nats/nsc/.config

COPY .profile $WORKDIR

ENTRYPOINT ["/bin/sh", "-l"]
