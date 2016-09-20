package cyclist

/* TODO: #5
type sqsHandler struct {
	queueURL    string
	concurrency int

	db     repo
	log    logrus.FieldLogger
	asSvc  autoscalingiface.AutoScalingAPI
	snsSvc snsiface.SNSAPI
	sqsSvc sqsiface.SQSAPI
}

func (sh *sqsHandler) Run(ctx context.Context) error {
	sh.log.WithField("queue_url", sh.queueURL).Debug("fetching queue attributes")

	params := &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(sh.queueURL),
		AttributeNames: []*string{
			aws.String("All"),
		},
	}

	resp, err := sh.sqsSvc.GetQueueAttributes(params)
	if err != nil {
		return err
	}

	sh.log.WithField("queue_attrs", resp.Attributes).Debug("fetched queue attributes")
	sh.log.WithField("concurrency", sh.concurrency).Debug("starting SQS consumers")

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
			sh.log.WithField("params", params).Debug("receiving")
		}

		resp, err := sh.sqsSvc.ReceiveMessage(params)
		if err != nil {
			ae := err.(awserr.Error)
			sh.log.WithFields(logrus.Fields{
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
	sh.log.WithField("msg", message).Debug("not really handling")
	return
}
*/
