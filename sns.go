package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/travis-ci/pudding/db"
)

type snsMessage struct {
	Message   string
	MessageID string `json:"MessageId"`
	Token     string
	TopicARN  string `json:"TopicArn"`
}

// lifecycleAction is an SNS message payload of the form:
// {
//   "AutoScalingGroupName":"name string",
//   "Service":"prose goop string",
//   "Time":"iso 8601 timestamp string",
//   "AccountId":"account id string",
//   "LifecycleTransition":"transition string, e.g.: autoscaling:EC2_INSTANCE_TERMINATING",
//   "RequestId":"uuid string",
//   "LifecycleActionToken":"uuid string",
//   "EC2InstanceId":"instance id string",
//   "LifecycleHookName":"name string"
// }
type lifecycleAction struct {
	Event                string
	AutoScalingGroupName string `redis:"auto_scaling_group_name"`
	Service              string
	Time                 string
	AccountID            string `json:"AccountId"`
	LifecycleTransition  string
	RequestID            string `json:"RequestId"`
	LifecycleActionToken string `redis:"lifecycle_action_token"`
	EC2InstanceID        string `json:"EC2InstanceId"`
	LifecycleHookName    string `redis:"lifecycle_hook_name"`
}

func (m *snsMessage) lifecycleAction() (*lifecycleAction, error) {
	a := &lifecycleAction{}
	err := json.Unmarshal([]byte(m.Message), a)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func handleSNSConfirmation(msg *snsMessage, awsRegion string) error {
	svc := sns.New(
		session.New(),
		&aws.Config{
			Region: aws.String(awsRegion),
		})

	params := &sns.ConfirmSubscriptionInput{
		Token:    aws.String(msg.Token),
		TopicArn: aws.String(msg.TopicARN),
	}
	_, err := svc.ConfirmSubscription(params)
	return err
}

func handleSNSNotification(msg *snsMessage, awsRegion string) error {
	action, err := msg.lifecycleAction()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "invalid json received in sns Message: %s\n", err)
		return
	}

	if action.Event == "autoscaling:TEST_NOTIFICATION" {
		w.WriteHeader(http.StatusOK)
		logrus.WithField("event", action.Event).Info("ignoring")
		return
	}

	switch action.LifecycleTransition {
	case "autoscaling:EC2_INSTANCE_LAUNCHING":
		logrus.WithField("action", action).Debug("storing instance launching lifecycle action")
		return db.StoreInstanceLifecycleAction(rc, action)
	case "autoscaling:EC2_INSTANCE_TERMINATING":
		logrus.WithField("action", action).Debug("setting expected_state to down")
		err = db.SetInstanceAttributes(rc, a.EC2InstanceID, map[string]string{"expected_state": "down"})
		if err != nil {
			return err
		}
		logrus.WithField("action", action).Debug("storing instance terminating lifecycle action")
		return db.StoreInstanceLifecycleAction(rc, action)
	default:
		logrus.WithField("action", a).Warn("unable to handle unknown lifecycle transition")
	}
}

func newSnsHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		msg := &snsMessage{}
		err := json.NewDecoder(r.Body).Decode(msg)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "invalid json received: %s\n", err)
			return
		}

		switch msg.Type {
		case "SubscriptionConfirmation":
			err = handleSNSConfirmation(msg, awsRegion)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "sns confirmation failed: %s\n", err)
				return
			}
		case "Notification":
			err = handleSNSNotification(msg, awsRegion)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "sns notification processing failed: %s\n", err)
				return
			}
		}
	}
}
