package cyclist

import (
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var (
	transitionHandlers = map[string]func(repo, string) error{
		"launching":   handleLaunchingLifecycleTransition,
		"terminating": handleTerminatingLifecycleTransition,
	}
)

func handleLaunchingLifecycleTransition(db repo, instanceID string) error {
	err := db.setInstanceState(instanceID, "up")
	if err != nil {
		return err
	}

	return db.storeInstanceEvent(instanceID, "launching")
}

func handleTerminatingLifecycleTransition(db repo, instanceID string) error {
	err := db.wipeInstanceState(instanceID)
	if err != nil {
		return err
	}

	return db.storeInstanceEvent(instanceID, "terminating")
}

func handleLifecycleTransition(db repo, log logrus.FieldLogger,
	asSvc autoscalingiface.AutoScalingAPI, transition, instanceID string) error {

	log = log.WithFields(logrus.Fields{
		"instance":   instanceID,
		"transition": transition,
	})

	action, err := db.fetchInstanceLifecycleAction(transition, instanceID)
	if err != nil {
		return err
	}

	if action == nil {
		return fmt.Errorf("no lifecycle transition '%s' for instance '%s'",
			transition, instanceID)
	}

	if action.Completed {
		log.Info("already completed")
		return nil
	}

	input := &autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  aws.String(action.AutoScalingGroupName),
		InstanceId:            aws.String(instanceID),
		LifecycleActionResult: aws.String("CONTINUE"),
		LifecycleActionToken:  aws.String(action.LifecycleActionToken),
		LifecycleHookName:     aws.String(action.LifecycleHookName),
	}
	_, err = asSvc.CompleteLifecycleAction(input)
	if err != nil {
		return err
	}

	err = db.completeInstanceLifecycleAction(transition, instanceID)
	if err != nil {
		log.WithField("err", err).Warn("failed to set lifecycle action bits")
	}

	if transitionHandler, ok := transitionHandlers[transition]; ok {
		log.Info("sending to transition handler")
		return transitionHandler(db, instanceID)
	}

	return fmt.Errorf("unknown lifecycle transition '%s'", transition)
}

func newLifecycleHandlerFunc(transition string, db repo,
	log logrus.FieldLogger,
	asSvc autoscalingiface.AutoScalingAPI) http.HandlerFunc {

	gerund := (map[string]string{
		"launch":      "launching",
		"termination": "terminating",
	})[transition]

	return func(w http.ResponseWriter, r *http.Request) {
		log = log.WithFields(logrus.Fields{
			"path":   r.URL.Path,
			"method": r.Method,
		})
		err := handleLifecycleTransition(
			db, log, asSvc, gerund, mux.Vars(r)["instance_id"])
		if err != nil {
			log.WithField("err", err).Error("handling lifecycle transition failed")
			jsonRespond(w, http.StatusBadRequest, &jsonErr{
				Err: errors.Wrap(err, "handling lifecycle transition failed"),
			})
			return
		}

		jsonRespond(w, http.StatusOK, &jsonMsg{
			Message: fmt.Sprintf("instance %s complete", transition),
		})
	}
}

func newLifecycleEventsHandlerFunc(db repo, log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log = log.WithFields(logrus.Fields{
			"path":   r.URL.Path,
			"method": r.Method,
		})

		instanceID := mux.Vars(r)["instance_id"]

		events, err := db.fetchInstanceEvents(instanceID)
		if err != nil {
			log.WithField("err", err).Error("fetching lifecycle events failed")
			jsonRespond(w, http.StatusBadRequest, &jsonErr{
				Err: errors.Wrap(err, "fetching lifecycle events failed"),
			})
			return
		}

		jsonRespond(w, http.StatusOK, &jsonLifecycleEvents{
			Data: events,
			Meta: map[string]string{
				"instance_id": instanceID,
			},
		})
	}
}

type jsonLifecycleEvents struct {
	Data []*lifecycleEvent `json:"data"`
	Meta map[string]string `json:"meta"`
}
