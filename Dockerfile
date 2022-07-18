FROM golang:1.18-alpine AS builder

LABEL maintainer "Derek Collison <derek@nats.io>"
LABEL maintainer "Waldemar Quevedo <wally@nats.io>"
LABEL maintainer "Jaime Piña <jaime@nats.io>"

WORKDIR $GOPATH/src/github.com/nats-io/

RUN apk add -U --no-cache git binutils

RUN go install github.com/nats-io/nats-top@v0.4.0

RUN go install -ldflags "-X main.version=2.7.1" github.com/nats-io/nsc@2.7.1

RUN mkdir -p src/github.com/nats-io && \
    cd src/github.com/nats-io/ && \
    git clone https://github.com/nats-io/natscli.git && \
    cd natscli/nats && \
    git fetch origin && \
    git checkout v0.0.33 && \
    go build -ldflags "-s -w -X main.version=0.0.33" -o /nats

RUN go install github.com/nats-io/stan.go/examples/stan-pub@latest
RUN go install github.com/nats-io/stan.go/examples/stan-sub@latest
RUN go install github.com/nats-io/stan.go/examples/stan-bench@latest

FROM alpine:3.14.6

RUN apk add -U --no-cache ca-certificates figlet

COPY --from=builder /go/bin/* /usr/local/bin/
COPY --from=builder /nats /usr/local/bin/

RUN cd /usr/local/bin/ && \
    ln -s nats-box nats-pub && \
    ln -s nats-box nats-sub && \
    ln -s nats-box nats-req && \
    ln -s nats-box nats-rply

WORKDIR /root

USER root

ENV NKEYS_PATH /nsc/nkeys
ENV XDG_DATA_HOME /nsc
ENV XDG_CONFIG_HOME /nsc/.config

COPY .profile $WORKDIR

CMD ["/bin/sh", "-l"]
