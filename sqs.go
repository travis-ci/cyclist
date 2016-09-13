package cyclist

import (
	"context"
	"crypto/rand"
	"math/big"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
)

type sqsHandler struct {
	queueURL    string
	concurrency int

	db repo
	l  *logrus.Logger
	a  autoscalingiface.AutoScalingAPI
	n  snsiface.SNSAPI
	s  sqsiface.SQSAPI
}

func (sh *sqsHandler) Run(ctx context.Context) error {
	sh.l.WithField("queue_url", sh.queueURL).Debug("fetching queue attributes")

	params := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(sh.queueURL),
		AttributeNames: []*string{
			aws.String("All"),
		},
	}

	resp, err := sh.s.GetQueueAttributes(params)
	if err != nil {
		return err
	}

	sh.l.WithField("queue_attrs", resp.Attributes).Debug("fetched queue attributes")
	sh.l.WithField("concurrency", sh.concurrency).Debug("starting SQS consumers")

	wg := &sync.WaitGroup{}

	for i := sh.concurrency; i > -1; i-- {
		wg.Add(1)
		sleepMs, err := rand.Int(rand.Reader, big.NewInt(5000))
		if err != nil {
			return err
		}
		time.Sleep(time.Duration(sleepMs.Int64()) * time.Millisecond)
		go sh.runOne(wg, ctx)
	}

	wg.Wait()
	return nil
}

func (sh *sqsHandler) runOne(wg *sync.WaitGroup, ctx context.Context) {
	params := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(sh.queueURL),
		AttributeNames: []*string{
			aws.String("All"),
		},
		MaxNumberOfMessages: aws.Int64(1),
		MessageAttributeNames: []*string{
			aws.String("All"),
		},
		VisibilityTimeout: aws.Int64(1),
		WaitTimeSeconds:   aws.Int64(1),
	}

	for {
		select {
		case <-ctx.Done():
			wg.Done()
			return
		default:
			sh.l.WithField("params", params).Debug("receiving")
		}

		resp, err := sh.s.ReceiveMessage(params)
		if err != nil {
			ae := err.(awserr.Error)
			sh.l.WithFields(logrus.Fields{
				"errcode": ae.Code(),
				"errmsg":  ae.Message(),
				"err":     ae.OrigErr(),
			}).Error("failed to receive from SQS queue")
			continue
		}

		if resp.Messages != nil {
			for _, message := range resp.Messages {
				sh.handle(message)
			}
		}
	}
}

func (sh *sqsHandler) handle(message *sqs.Message) {
	sh.l.WithField("msg", message).Debug("not really handling")
	return
}
