FROM golang:1.9 as builder
MAINTAINER Travis CI GmbH <support+cyclist-docker-image@travis-ci.org>

RUN go get -u github.com/FiloSottile/gvt
COPY . /go/src/github.com/travis-ci/cyclist
WORKDIR /go/src/github.com/travis-ci/cyclist
ENV CGO_ENABLED 0
#RUN make deps
RUN make

#################################
### linux/amd64/cyclist ###
#################################

#FROM alpine:latest
#RUN apk --no-cache add ca-certificates curl bash

#COPY --from=builder /go/bin/cyclist /usr/local/bin/cyclist
#COPY --from=builder /go/src/github.com/travis-ci/cyclist/.docker-entrypoint.sh /docker-entrypoint.sh

#VOLUME ["/var/tmp"]
#STOPSIGNAL SIGINT

#ENTRYPOINT ["/docker-entrypoint.sh"]
#CMD ["/usr/local/bin/cyclist"]
