#!/bin/bash
if [[ "${DEVELOPMENT}" == "true" ]]; then
    make dev-server
else
    cyclist serve --port "${PORT:=9753}" "$@"
fi
