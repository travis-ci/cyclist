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

func newSNSHandlerFunc(db repo, log logrus.FieldLogger, snsSvc snsiface.SNSAPI, snsVerify bool, tokGen tokenGenerator) http.HandlerFunc {
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

		err = nil
		status := http.StatusBadRequest

		switch msg.Type {
		case "SubscriptionConfirmation":
			status, err = handleSNSSubscriptionConfirmation(snsSvc, msg)
		case "Notification":
			status, err = handleSNSNotification(db, log, tokGen, msg)
		default:
			log.WithField("type", msg.Type).Warn("unknown sns message type")
			jsonRespond(w, http.StatusBadRequest, map[string]interface{}{
				"error": fmt.Sprintf("unknown message type '%s'", msg.Type),
			})
			return
		}

		if err != nil {
			log.WithFields(logrus.Fields{
				"err":  err,
				"type": msg.Type,
			}).Error("failed to handle sns message")
			jsonRespond(w, status, &jsonErr{Err: err})
			return
		}

		jsonRespond(w, status, &jsonMsg{
			Message: fmt.Sprintf("handled '%s' message", msg.Type),
		})
	}
}

func handleSNSSubscriptionConfirmation(snsSvc snsiface.SNSAPI, msg *snsMessage) (int, error) {
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

func handleSNSNotification(db repo, log logrus.FieldLogger, tokGen tokenGenerator, msg *snsMessage) (int, error) {
	la, err := msg.lifecycleAction()
	if err != nil {
		return http.StatusBadRequest, errors.Wrap(err, "invalid json received in sns Message")
	}

	if la == nil {
		return http.StatusBadRequest, errors.New("no lifecycle action present in sns Message")
	}

	if la.Event == "autoscaling:TEST_NOTIFICATION" {
		log.WithField("event", la.Event).Debug("ignoring")
		return http.StatusAccepted, nil
	}

	err = nil

	switch la.LifecycleTransition {
	case "autoscaling:EC2_INSTANCE_LAUNCHING":
		err = handleAutoScalingInstanceLaunching(db, log, tokGen, la)
	case "autoscaling:EC2_INSTANCE_TERMINATING":
		err = handleAutoScalingInstanceTerminating(db, log, la)
	default:
		log.WithField("transition", la.LifecycleTransition).Warn("unknown lifecycle transition")
		return http.StatusBadRequest, fmt.Errorf("unknown lifecycle transition %q", la.LifecycleTransition)
	}

	if err != nil {
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func handleAutoScalingInstanceTerminating(db repo, log logrus.FieldLogger, la *lifecycleAction) error {
	log.WithField("action", la).Debug("setting expected_state to down")
	err := db.setInstanceState(la.EC2InstanceID, "down")
	if err != nil {
		return err
	}
	log.WithField("action", la).Debug("storing instance terminating lifecycle action")
	err = db.storeInstanceLifecycleAction(la)
	if err != nil {
		return err
	}
	return db.storeInstanceEvent(la.EC2InstanceID, "preterminating")
}

func handleAutoScalingInstanceLaunching(db repo, log logrus.FieldLogger, tokGen tokenGenerator, la *lifecycleAction) error {
	log.WithField("action", la).Debug("storing instance launching lifecycle action")
	err := db.storeInstanceToken(la.EC2InstanceID, tokGen.GenerateToken())
	if err != nil {
		return err
	}
	err = db.storeInstanceLifecycleAction(la)
	if err != nil {
		return err
	}
	log.WithField("action", la).Debug("setting expected_state to up")
	err = db.setInstanceState(la.EC2InstanceID, "up")
	if err != nil {
		return err
	}
	return db.storeInstanceEvent(la.EC2InstanceID, "prelaunching")
}
