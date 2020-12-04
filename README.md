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

 * nats     - NATS management utility ([README](https://github.com/nats-io/natscli#readme))
 * nsc      - create NATS accounts and users
 * nats-top - top-like tool for monitoring NATS servers

## Getting started

Use tools to interact with NATS.

```
$ docker run --rm -it synadia/nats-box:latest
~ # nats pub -s demo.nats.io test 'Hello World'
16:33:27 Published 11 bytes to "test"
```

Running in Kubernetes:

```sh
# Interactive mode
kubectl run -i --rm --tty nats-box --image=synadia/nats-box --restart=Never
nats-box:~# nats sub -s nats hello &
nats-box:~# nats pub -s nats hello world

# Non-interactive mode
kubectl apply -f https://nats-io.github.io/k8s/tools/nats-box.yml
kubectl exec -it nats-box -- /bin/sh
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
