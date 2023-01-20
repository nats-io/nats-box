#!/bin/sh

if [[ "${HOME}" != "/root" && "${PWD}" == "/root" ]]; then
  cd "${HOME}"
fi

if [[ $# -ne 0 ]]; then
  exec "$@"
else
  exec /bin/sh -l
fi
