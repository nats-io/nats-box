# nats-box

A container with NATS utilities

* nats-pub - publish NATS messages
* nats-req - send a request, receive a response over NATS
* nats-sub - subscribe to NATS messages on a topic
* nats-top - top-like tool for monitoring NATS servers
* nsc      - create NATS accounts and users

## Usage

Use tools to interact with NATS.

```
$ docker run --rm -it synadia/nats-box:latest
~ # nats-pub -s 1.2.3.4:4222 foo hello
Published [foo] : 'hello'
```

Mount a volume to get nsc accounts, nkeys, and other config back on the host.

```
$ docker run --rm -it -v $(pwd)/nsc:/nsc synadia/nats-box:latest
~ # export NKEYS_PATH=/nsc/nkeys
~ # export NSC_HOME=/nsc/accounts
~ # export NATS_CONFIG_HOME=/nsc/config
~ # nsc init
~ # chmod -R 1000:1000 /nsc
$ tree -L 2 nsc/
nsc/
├── accounts
│   ├── nats
│   └── nsc.json
└── nkeys
    ├── creds
    └── keys

5 directories, 1 file
```
