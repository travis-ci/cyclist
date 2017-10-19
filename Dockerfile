FROM golang:1.9 as builder
MAINTAINER Travis CI GmbH <support+cyclist-docker-image@travis-ci.org>

RUN go get -u github.com/alecthomas/gometalinter
RUN go get -u github.com/aws/aws-sdk-go/aws
RUN go get -u github.com/aws/aws-sdk-go/aws/session
RUN go get -u github.com/aws/aws-sdk-go/service/autoscaling
RUN go get -u github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface
RUN go get -u github.com/aws/aws-sdk-go/service/sns
RUN go get -u github.com/aws/aws-sdk-go/service/sns/snsiface
RUN go get -u github.com/garyburd/redigo/redis
RUN go get -u github.com/gorilla/mux
RUN go get -u github.com/meatballhat/negroni-logrus
RUN go get -u github.com/pborman/uuid
RUN go get -u github.com/pkg/errors
RUN go get -u github.com/sirupsen/logrus
RUN go get -u github.com/urfave/negroni
RUN go get -u github.com/FiloSottile/gvt
RUN go get -u gopkg.in/urfave/cli.v2


COPY . /go/src/github.com/travis-ci/cyclist
WORKDIR /go/src/github.com/travis-ci/cyclist
RUN make deps
ENV CGO_ENABLED 0
RUN rm -rf vendor/github.com/meatballhat/
RUN make build

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
