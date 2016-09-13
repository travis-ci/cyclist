package cyclist

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type sqsHandler struct {
	queueURL    string
	concurrency int
}

func newSqsHandler(queueURL string, concurrency int) *sqsHandler {
	return &sqsHandler{queueURL: queueURL, concurrency: concurrency}
}

func (sl *sqsHandler) Run(ctx context.Context) error {
	log.WithField("queue_url", sl.queueURL).Debug("fetching queue attributes")

	svc := sqs.New(session.New())
	params := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(sl.queueURL),
		AttributeNames: []*string{
			aws.String("All"),
		},
	}

	resp, err := svc.GetQueueAttributes(params)
	if err != nil {
		return err
	}

	log.WithField("queue_attrs", resp.Attributes).Debug("fetched queue attributes")
	log.WithField("concurrency", sl.concurrency).Debug("starting SQS consumers")

	wg := &sync.WaitGroup{}

	for i := sl.concurrency; i > -1; i-- {
		wg.Add(1)
		go sl.runOne(wg, ctx)
	}

	wg.Wait()
	return nil
}

func (sl *sqsHandler) runOne(wg *sync.WaitGroup, ctx context.Context) {
	svc := sqs.New(session.New())
	params := &sqs.ReceiveMessageInput{
		QueueUrl: aws.String(sl.queueURL),
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
			log.WithField("params", params).Debug("receiving")
		}

		resp, err := svc.ReceiveMessage(params)
		if err == nil {
			log.WithField("err", err).Error("failed to receive from SQS queue")
			continue
		}

		handleSQSMessage(resp)
	}
}

func handleSQSMessage(resp *sqs.ReceiveMessageOutput) {
	log.WithField("msg", resp).Debug("not really handling")
	return
}
