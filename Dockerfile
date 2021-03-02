FROM golang:1.16-alpine AS builder

LABEL maintainer "Derek Collison <derek@nats.io>"
LABEL maintainer "Waldemar Quevedo <wally@nats.io>"
LABEL maintainer "Jaime Piña <jaime@nats.io>"

WORKDIR $GOPATH/src/github.com/nats-io/

RUN apk add -U --no-cache git binutils

RUN go get github.com/nats-io/nats-top

RUN go get -u -ldflags "-X main.version=$(date +%Y%m%d)" github.com/nats-io/nsc@master

RUN mkdir -p src/github.com/nats-io && \
    cd src/github.com/nats-io/ && \
    git clone https://github.com/nats-io/natscli.git && \
    cd natscli/nats && \
    go build -ldflags "-s -w -X main.version=$(date +%Y%m%d)" -o /nats

RUN go get github.com/nats-io/stan.go/examples/stan-pub
RUN go get github.com/nats-io/stan.go/examples/stan-sub
RUN go get github.com/nats-io/stan.go/examples/stan-bench

# Simple tools
COPY . .
RUN go install
RUN strip /go/bin/*

FROM alpine:3.13

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
ENV NSC_HOME /nsc/accounts
ENV NATS_CONFIG_HOME /nsc/config

COPY .profile $WORKDIR

ENTRYPOINT ["/bin/sh", "-l"]
