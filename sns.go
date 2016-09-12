package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/pkg/errors"
)

func handleSNSConfirmation(msg *snsMessage, awsRegion string) (int, error) {
	svc := sg.Get(awsRegion)

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

func handleSNSNotification(msg *snsMessage, awsRegion string) (int, error) {
	action, err := msg.lifecycleAction()
	if err != nil {
		return http.StatusBadRequest, errors.Wrap(err, "invalid json received in sns Message")
	}

	if action.Event == "autoscaling:TEST_NOTIFICATION" {
		log.WithField("event", action.Event).Info("ignoring")
		return http.StatusAccepted, nil
	}

	switch action.LifecycleTransition {
	case "autoscaling:EC2_INSTANCE_LAUNCHING":
		log.WithField("action", action).Debug("storing instance launching lifecycle action")
		rc := dbPool.Get()

		err = storeInstanceLifecycleAction(rc, action)
		if err != nil {
			return http.StatusBadRequest, err
		}
		return http.StatusOK, nil
	case "autoscaling:EC2_INSTANCE_TERMINATING":
		log.WithField("action", action).Debug("setting expected_state to down")
		rc := dbPool.Get()

		err = setInstanceState(rc, action.EC2InstanceID, "down")
		if err != nil {
			return http.StatusBadRequest, err
		}
		log.WithField("action", action).Debug("storing instance terminating lifecycle action")
		err = storeInstanceLifecycleAction(rc, action)
		if err != nil {
			return http.StatusBadRequest, err
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
			status, err := handleSNSNotification(msg, awsRegion)
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
