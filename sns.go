package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/pkg/errors"
)

type snsMessage struct {
	Message   string
	MessageID string `json:"MessageId"`
	Token     string
	TopicARN  string `json:"TopicArn"`
	Type      string
}

func (m *snsMessage) lifecycleAction() (*lifecycleAction, error) {
	a := &lifecycleAction{}
	err := json.Unmarshal([]byte(m.Message), a)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func handleSNSConfirmation(msg *snsMessage, awsRegion string) (int, error) {
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
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

func handleSNSNotification(w http.ResponseWriter, msg *snsMessage, awsRegion string) (int, error) {
	action, err := msg.lifecycleAction()
	if err != nil {
		return http.StatusBadRequest, errors.Wrap(err, "invalid json received in sns Message")
	}

	if action.Event == "autoscaling:TEST_NOTIFICATION" {
		logrus.WithField("event", action.Event).Info("ignoring")
		return http.StatusOK, nil
	}

	rc := dbPool.Get()

	switch action.LifecycleTransition {
	case "autoscaling:EC2_INSTANCE_LAUNCHING":
		logrus.WithField("action", action).Debug("storing instance launching lifecycle action")
		err = storeInstanceLifecycleAction(rc, action)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusOK, nil
	case "autoscaling:EC2_INSTANCE_TERMINATING":
		logrus.WithField("action", action).Debug("setting expected_state to down")
		err = setInstanceAttributes(rc, action.EC2InstanceID, map[string]string{"expected_state": "down"})
		if err != nil {
			return http.StatusInternalServerError, err
		}
		logrus.WithField("action", action).Debug("storing instance terminating lifecycle action")
		err = storeInstanceLifecycleAction(rc, action)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		return http.StatusOK, nil
	default:
		return http.StatusBadRequest, fmt.Errorf("unknown lifecycle transition %q", action.LifecycleTransition)
	}
}

func newSnsHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		msg := &snsMessage{}
		err := json.NewDecoder(r.Body).Decode(msg)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid json received: %s"}`, err)
			return
		}

		switch msg.Type {
		case "SubscriptionConfirmation":
			status, err := handleSNSConfirmation(msg, awsRegion)
			body := `{"ok": true}`
			if err != nil {
				body = fmt.Sprintf(`{"error":%q}`, err.Error())
			}
			w.WriteHeader(status)
			fmt.Fprintf(w, body)
			return
		case "Notification":
			status, err := handleSNSNotification(w, msg, awsRegion)
			body := `{"ok": true}`
			if err != nil {
				body = fmt.Sprintf(`{"error":%q}`, err.Error())
			}
			w.WriteHeader(status)
			fmt.Fprintf(w, body)
			return
		default:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"unknown message type '%s'"}`, msg.Type)
			return
		}
	}
}
