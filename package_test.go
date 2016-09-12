package cyclist

import (
	"errors"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/garyburd/redigo/redis"
	"github.com/rafaeljusto/redigomock"
)

func init() {
	if log == nil {
		log = logrus.New()
	}
	log.Level = logrus.FatalLevel
}

type testSNSGetter struct {
	ErrorConfirmSubscription bool
}

func (tsg *testSNSGetter) Get(awsRegion string) *sns.SNS {
	svc := sns.New(session.New(), &aws.Config{Region: aws.String(awsRegion)})
	svc.Handlers.Clear()
	svc.Handlers.Build.PushBack(func(r *request.Request) {
		if tsg.ErrorConfirmSubscription {
			r.Error = errors.New("boom")
		}
	})
	return svc
}

type testRedisConnGetter struct {
	Conn *redigomock.Conn
}

func (trcg *testRedisConnGetter) Get() redis.Conn {
	if trcg.Conn == nil {
		trcg.Conn = redigomock.NewConn()
	}
	return trcg.Conn
}
