package cyclist

import (
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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
	asSvc autoscalingiface.AutoScalingAPI, detach bool, transition,
	instanceID string) error {

	log = log.WithFields(logrus.Fields{
		"transition": transition,
		"detach":     detach,
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

	if transition == "terminating" && detach {
		err = detachInstanceFromASG(action, log, asSvc)
	} else {
		err = completeLifecycleAction(action, log, asSvc)
	}

	if err != nil {
		return err
	}

	err = db.completeInstanceLifecycleAction(transition, instanceID)
	if err != nil {
		log.WithField("err", err).Warn("failed to set lifecycle action bits")
	}

	switch transition {
	case "launching":
		log.Info("sending to transition handler")
		return handleLaunchingLifecycleTransition(db, instanceID)
	case "terminating":
		log.Info("sending to transition handler")
		return handleTerminatingLifecycleTransition(db, instanceID)
	default:
		return fmt.Errorf("unknown lifecycle transition '%s'", transition)
	}
}

func detachInstanceFromASG(la *lifecycleAction, log logrus.FieldLogger, asSvc autoscalingiface.AutoScalingAPI) error {
	log.WithFields(logrus.Fields{
		"asg":       la.AutoScalingGroupName,
		"hook_name": la.LifecycleHookName,
		"instance":  la.EC2InstanceID,
	}).Info("detaching instance from asg")

	input := &autoscaling.DetachInstancesInput{
		AutoScalingGroupName:           aws.String(la.AutoScalingGroupName),
		InstanceIds:                    aws.StringSlice([]string{la.EC2InstanceID}),
		ShouldDecrementDesiredCapacity: aws.Bool(false),
	}

	_, err := asSvc.DetachInstances(input)
	return err
}

func completeLifecycleAction(la *lifecycleAction, log logrus.FieldLogger, asSvc autoscalingiface.AutoScalingAPI) error {
	log.WithFields(logrus.Fields{
		"asg":       la.AutoScalingGroupName,
		"hook_name": la.LifecycleHookName,
	}).Info("completing lifecycle action")

	input := &autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  aws.String(la.AutoScalingGroupName),
		InstanceId:            aws.String(la.EC2InstanceID),
		LifecycleActionResult: aws.String("CONTINUE"),
		LifecycleActionToken:  aws.String(la.LifecycleActionToken),
		LifecycleHookName:     aws.String(la.LifecycleHookName),
	}
	_, err := asSvc.CompleteLifecycleAction(input)
	return err
}

func newLifecycleHandlerFunc(transition string, db repo,
	log logrus.FieldLogger,
	asSvc autoscalingiface.AutoScalingAPI,
	detach bool) http.HandlerFunc {

	gerund := (map[string]string{
		"launch":      "launching",
		"termination": "terminating",
	})[transition]

	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := mux.Vars(r)["instance_id"]
		log = log.WithFields(logrus.Fields{
			"path":     r.URL.Path,
			"method":   r.Method,
			"instance": instanceID,
		})
		err := handleLifecycleTransition(
			db, log, asSvc, detach, gerund, instanceID)
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

func newImplosionsHandlerFunc(db repo, log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID := mux.Vars(r)["instance_id"]
		log = log.WithField("instance", instanceID)
		err := db.setInstanceState(instanceID, "down")
		if err != nil {
			log.WithField("err", err).Error("setting instance state down failed")
			jsonRespond(w, http.StatusInternalServerError, &jsonErr{
				Err: errors.Wrap(err, "setting instance state down failed"),
			})
			return
		}
		err = db.storeInstanceEvent(instanceID, "implosion")
		if err != nil {
			log.WithField("err", err).Error("storing implosion event failed")
			jsonRespond(w, http.StatusInternalServerError, &jsonErr{
				Err: errors.Wrap(err, "storing implosion event failed"),
			})
			return
		}

		jsonRespond(w, http.StatusOK, &jsonMsg{
			Message: fmt.Sprintf("instance implosion recorded"),
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
			Events:     events,
			InstanceID: instanceID,
		})
	}
}

func newAllLifecycleEventsHandlerFunc(db repo, log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log = log.WithFields(logrus.Fields{
			"path":   r.URL.Path,
			"method": r.Method,
		})

		events, err := db.fetchAllInstanceEvents()
		if err != nil {
			log.WithField("err", err).Error("fetching all lifecycle events failed")
			jsonRespond(w, http.StatusBadRequest, &jsonErr{
				Err: errors.Wrap(err, "fetching all lifecycle events failed"),
			})
			return
		}

		jsonRespond(w, http.StatusOK, &jsonAllLifecycleEvents{
			Events: events,
			Total:  len(events),
		})
	}
}

type jsonLifecycleEvents struct {
	Events     []*lifecycleEvent `json:"events"`
	InstanceID string            `json:"@instance_id"`
}

type jsonAllLifecycleEvents struct {
	Events map[string][]*lifecycleEvent `json:"events"`
	Total  int                          `json:"@total"`
}
