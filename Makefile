PACKAGES := \
	github.com/travis-ci/cyclist \
	github.com/travis-ci/cyclist/cmd/travis-cyclist

VERSION_VAR := github.com/travis-ci/cyclist.VersionString
VERSION_VALUE ?= $(shell git describe --always --dirty --tags 2>/dev/null)
REV_VAR := github.com/travis-ci/cyclist.RevisionString
REV_VALUE ?= $(shell git rev-parse HEAD 2>/dev/null || echo "???")
REV_URL_VAR := github.com/travis-ci/cyclist.RevisionURLString
REV_URL_VALUE ?= https://github.com/travis-ci/cyclist/tree/$(REV_VALUE)
GENERATED_VAR := github.com/travis-ci/cyclist.GeneratedString
GENERATED_VALUE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%S%z')
COPYRIGHT_VAR := github.com/travis-ci/cyclist.CopyrightString
COPYRIGHT_VALUE ?= $(shell grep -i ^copyright LICENSE | sed 's/^[Cc]opyright //')

GOPATH := $(shell echo "$${GOPATH%%:*}")
GOBUILD_LDFLAGS ?= \
	-X '$(VERSION_VAR)=$(VERSION_VALUE)' \
	-X '$(REV_VAR)=$(REV_VALUE)' \
	-X '$(REV_URL_VAR)=$(REV_URL_VALUE)' \
	-X '$(GENERATED_VAR)=$(GENERATED_VALUE)' \
	-X '$(COPYRIGHT_VAR)=$(COPYRIGHT_VALUE)'

BINARY_NAMES := $(notdir $(wildcard cmd/*))
BINARIES := $(addprefix bin/,$(BINARY_NAMES))

.PHONY: all
all: clean deps lint test bin

.PHONY: test
test: deps
	go test -ldflags "$(GOBUILD_LDFLAGS)" $(PACKAGES)

.PHONY: list-deps
list-deps:
	@go list -f '{{ join .Imports "\n" }}' $(PACKAGES) | sort | uniq

.PHONY: lint
lint: deps
	gometalinter --deadline=1m -Dstructcheck -Derrcheck -Dgotype --vendor . ./cmd/*/

.PHONY: bin
bin: deps $(BINARIES)

.PHONY: clean
clean:
	find $(GOPATH)/pkg -wholename '*travis-ci/cyclist*.a' | xargs $(RM)
	$(RM) $(BINARIES)

deps: $(GOPATH)/bin/gvt vendor/.deps-fetched

vendor/.deps-fetched:
	gvt rebuild
	touch $@

bin/%: cmd/% $(wildcard **/*.go)
	go build -ldflags "$(GOBUILD_LDFLAGS)" -o $@ ./$<

.PHONY: dev-server
dev-server: $(GOPATH)/bin/reflex
	reflex -r '\.go$$' -s go run ./cmd/travis-cyclist/main.go serve

$(GOPATH)/bin/gvt:
	go get github.com/FiloSottile/gvt

$(GOPATH)/bin/reflex:
	go get github.com/cespare/reflex
