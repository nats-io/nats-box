```
             _             _
 _ __   __ _| |_ ___      | |__   _____  __
| '_ \ / _` | __/ __|_____| '_ \ / _ \ \/ /
| | | | (_| | |_\__ \_____| |_) | (_) >  <
|_| |_|\__,_|\__|___/     |_.__/ \___/_/\_\
```

[![License][License-Image]][License-Url]
[![Version](https://d25lcipzij17d.cloudfront.net/badge.svg?id=go&type=5&v=0.4.0)](https://github.com/nats-io/nats-box/releases/tag/v0.4.0)

[License-Url]: https://www.apache.org/licenses/LICENSE-2.0
[License-Image]: https://img.shields.io/badge/License-Apache2-blue.svg

# nats-box

A lightweight container with NATS and NATS Streaming utilities.

* nats-pub - publish NATS messages
* nats-req - send a request, receive a response over NATS
* nats-sub - subscribe to NATS messages on a topic
* stan-pub - publish messages to NATS Streaming
* stan-sub - subscribe to messages from NATS Streaming
* nats-top - top-like tool for monitoring NATS servers
* nsc      - create NATS accounts and users
* nats     - NATS management utility (includes JetStream)

## Getting started

Use tools to interact with NATS.

```
$ docker run --rm -it synadia/nats-box:latest
~ # nats-pub -s demo.nats.io test 'Hello World'
Published [test] : 'Hello World'
```

Running in Kubernetes:

```sh
# Interactive mode
kubectl run -i --rm --tty nats-box --image=synadia/nats-box --restart=Never
nats-box:~# nats-sub -s nats hello &
nats-box:~# nats-pub -s nats hello world

# Non-interactive mode
kubectl apply -f https://nats-io.github.io/k8s/tools/nats-box.yml
kubectl exec -it nats-box
```

## Using NSC to manage NATS v2 users and accounts

You can mount a local volume to get nsc accounts, nkeys, and other config back on the host.

```
$ docker run --rm -it -v $(pwd)/nsc:/nsc synadia/nats-box:latest

# In case NSC not initialized already:
nats-box:~# nsc init -d /nsc
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
