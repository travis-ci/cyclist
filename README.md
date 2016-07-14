# cyclist 🚴

µService™ for managing AWS auto-scaling groups and their lifecycle events.

## Install

``` bash
go get github.com/FiloSottile/gvt
export GO15VENDOREXPERIMENT=1

make
```

## Develop

``` bash
go get github.com/cespare/reflex
reflex -r '\.go$' -s go run ./cmd/travis-cyclist/main.go
```
