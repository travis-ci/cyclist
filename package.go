package cyclist

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/sns"
	"gopkg.in/urfave/cli.v2"
)

var (
	// VersionString is a version!
	VersionString = "?"
	// RevisionString is a revision!
	RevisionString = "?"
	// RevisionURLString is a revision URL!
	RevisionURLString = "?"
	// GeneratedString is a timestamp!
	GeneratedString = "?"
	// CopyrightString is legalese!
	CopyrightString = "?"

	// RedisNamespace is the namespace used in redis OK!
	RedisNamespace = "cyclist"

	log *logrus.Logger

	sg snsGetter         = &defaultSNSGetter{}
	ag autoscalingGetter = &defaultAutoscalingGetter{}
)

func init() {
	cli.VersionPrinter = customVersionPrinter
}

func customVersionPrinter(ctx *cli.Context) {
	fmt.Fprintf(ctx.App.Writer, "%s v=%s rev=%s d=%s\n",
		ctx.App.Name, VersionString, RevisionString, GeneratedString)
}

type snsGetter interface {
	Get(awsRegion string) *sns.SNS
}

type defaultSNSGetter struct{}

func (dsg *defaultSNSGetter) Get(awsRegion string) *sns.SNS {
	return sns.New(
		session.New(),
		&aws.Config{
			Region: aws.String(awsRegion),
		})
}

type autoscalingGetter interface {
	Get(awsRegion string) *autoscaling.AutoScaling
}

type defaultAutoscalingGetter struct{}

func (dasg *defaultAutoscalingGetter) Get(awsRegion string) *autoscaling.AutoScaling {
	return autoscaling.New(
		session.New(),
		&aws.Config{
			Region: aws.String(awsRegion),
		})
}
