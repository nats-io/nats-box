```
             _             _
 _ __   __ _| |_ ___      | |__   _____  __
| '_ \ / _` | __/ __|_____| '_ \ / _ \ \/ /
| | | | (_| | |_\__ \_____| |_) | (_) >  <
|_| |_|\__,_|\__|___/     |_.__/ \___/_/\_\
```

[![License Apache 2.0](https://img.shields.io/badge/License-Apache2-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)
[![Docker Pulls](https://img.shields.io/docker/pulls/natsio/nats-box.svg?maxAge=604800)](https://hub.docker.com/r/natsio/nats-box)

# nats-box

A lightweight container with NATS utilities.

- nats - NATS management utility ([README](https://github.com/nats-io/natscli#readme))
- nsc - create NATS accounts and users ([README](https://github.com/nats-io/nsc#readme))
- nats-top - top-like tool for monitoring NATS servers ([README](https://github.com/nats-io/nats-top#readme))

## Getting started

Use tools to interact with NATS.

```
$ docker run --rm -it natsio/nats-box:latest
~ # nats pub -s demo.nats.io test 'Hello World'
16:33:27 Published 11 bytes to "test"
```

Running in Kubernetes:

```sh
# Interactive mode
kubectl run -i --rm --tty nats-box --image=natsio/nats-box --restart=Never
nats-box:~# nats sub -s nats hello &
nats-box:~# nats pub -s nats hello world

# Non-interactive mode
kubectl apply -f https://nats-io.github.io/k8s/tools/nats-box.yml
kubectl exec -it nats-box -- /bin/sh
```

## Using NSC to manage NATS v2 users and accounts

You can mount a local volume to get nsc accounts, nkeys, and other config back on the host.

```
$ docker run --rm -it -v $(pwd)/nsc:/nsc natsio/nats-box:latest

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

## Releasing

1. Create annotated tag, e.g.:

```
git tag -s -a "v0.19.0" -m "v0.19.0"
```

2. Github workflow will take care of the rest.
