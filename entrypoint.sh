#!/bin/sh

if [[ $# -ne 0 ]]; then
  exec "$@"
else
  exec /bin/sh -l
fi
