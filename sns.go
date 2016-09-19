package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/pkg/errors"
)

func newSnsHandlerFunc(db repo, log logrus.FieldLogger, snsSvc snsiface.SNSAPI, snsVerify bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log = log.WithFields(logrus.Fields{
			"path":   r.URL.Path,
			"method": r.Method,
		})
		msg := &snsMessage{}
		err := json.NewDecoder(r.Body).Decode(msg)
		if err != nil {
			log.WithField("err", err).Error("invalid json received")
			jsonRespond(w, http.StatusBadRequest, &jsonErr{
				Err: errors.Wrap(err, "invalid json received"),
			})
			return
		}

		if snsVerify {
			err = msg.verify()
			if err != nil {
				log.WithField("err", err).Error("failed to verify sns message")
				jsonRespond(w, http.StatusBadRequest, &jsonErr{
					Err: errors.Wrap(err, "failed to verify sns message"),
				})
			}
		}

		switch msg.Type {
		case "SubscriptionConfirmation":
			status, err := handleSNSConfirmation(snsSvc, msg)
			if err != nil {
				log.WithField("err", err).Error("failed to handle sns confirmation")
				jsonRespond(w, status, &jsonErr{Err: err})
				return
			}
			jsonRespond(w, status, &jsonMsg{
				Message: "subscription confirmed",
			})
			return
		case "Notification":
			status, err := handleSNSNotification(db, log, msg)
			if err != nil {
				log.WithField("err", err).Error("failed to handle sns notification")
				jsonRespond(w, status, &jsonErr{Err: err})
				return
			}
			jsonRespond(w, status, &jsonMsg{
				Message: "notification handled",
			})
			return
		default:
			log.WithField("type", msg.Type).Warn("unknown sns message type")
			jsonRespond(w, http.StatusBadRequest, map[string]interface{}{
				"error": fmt.Sprintf("unknown message type '%s'", msg.Type),
			})
			return
		}
	}
}

func handleSNSConfirmation(snsSvc snsiface.SNSAPI, msg *snsMessage) (int, error) {
	params := &sns.ConfirmSubscriptionInput{
		Token:    aws.String(msg.Token),
		TopicArn: aws.String(msg.TopicARN),
	}
	_, err := snsSvc.ConfirmSubscription(params)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

func handleSNSNotification(db repo, log logrus.FieldLogger, msg *snsMessage) (int, error) {
	action, err := msg.lifecycleAction()
	if err != nil {
		return http.StatusBadRequest, errors.Wrap(err, "invalid json received in sns Message")
	}

	if action == nil {
		return http.StatusBadRequest, errors.New("no lifecycle action present in sns Message")
	}

	if action.Event == "autoscaling:TEST_NOTIFICATION" {
		log.WithField("event", action.Event).Debug("ignoring")
		return http.StatusAccepted, nil
	}

	switch action.LifecycleTransition {
	case "autoscaling:EC2_INSTANCE_LAUNCHING":
		log.WithField("action", action).Debug("storing instance launching lifecycle action")
		err = db.storeInstanceLifecycleAction(action)
		if err != nil {
			return http.StatusBadRequest, err
		}
		log.WithField("action", action).Debug("setting expected_state to up")
		err = db.setInstanceState(action.EC2InstanceID, "up")
		if err != nil {
			return http.StatusBadRequest, err
		}
		err = db.storeInstanceEvent(action.EC2InstanceID, "prelaunching")
		if err != nil {
			return http.StatusBadRequest, err
		}
		return http.StatusOK, nil
	case "autoscaling:EC2_INSTANCE_TERMINATING":
		log.WithField("action", action).Debug("setting expected_state to down")
		err = db.setInstanceState(action.EC2InstanceID, "down")
		if err != nil {
			return http.StatusBadRequest, err
		}
		log.WithField("action", action).Debug("storing instance terminating lifecycle action")
		err = db.storeInstanceLifecycleAction(action)
		if err != nil {
			return http.StatusBadRequest, err
		}
		err = db.storeInstanceEvent(action.EC2InstanceID, "preterminating")
		if err != nil {
			return http.StatusBadRequest, err
		}
		return http.StatusOK, nil
	default:
		log.WithField("transition", action.LifecycleTransition).Warn("unknown lifecycle transition")
		return http.StatusBadRequest, fmt.Errorf("unknown lifecycle transition %q", action.LifecycleTransition)
	}
}
