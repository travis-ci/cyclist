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

func handleLifecycleTransition(db repo, log logrus.FieldLogger, asSvc autoscalingiface.AutoScalingAPI, transition, instanceID string) error {
	action, err := db.fetchInstanceLifecycleAction(transition, instanceID)
	if err != nil {
		return err
	}

	if action == nil {
		return fmt.Errorf("no lifecycle transition '%s' for instance '%s'", transition, instanceID)
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

	err = db.wipeInstanceLifecycleAction(transition, instanceID)
	if err != nil {
		log.WithField("err", err).Warn("failed to clean up lifecycle action bits")
	}

	switch transition {
	case "launching":
		err = db.setInstanceState(instanceID, "up")
		if err != nil {
			return err
		}
	case "terminating":
		err = db.wipeInstanceState(instanceID)
		if err != nil {
			return err
		}
	}

	return nil
}

func newInstanceLifecycleHandlerFunc(transition string, db repo, log logrus.FieldLogger, asSvc autoscalingiface.AutoScalingAPI) http.HandlerFunc {
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
