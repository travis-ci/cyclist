package cyclist

import (
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/gorilla/mux"
)

func handleLifecycleTransition(db repo, log *logrus.Logger, asSvc autoscalingiface.AutoScalingAPI, transition, instanceID string) error {
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

	return nil
}

func newInstanceLaunchHandlerFunc(db repo, log *logrus.Logger, asSvc autoscalingiface.AutoScalingAPI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		err := handleLifecycleTransition(
			db, log, asSvc, "launching", mux.Vars(r)["instance_id"])

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"handling lifecycle transition failed: %s"}`, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"instance launch complete"}`)
	}
}

func newInstanceTerminationHandlerFunc(db repo, log *logrus.Logger, asSvc autoscalingiface.AutoScalingAPI) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		err := handleLifecycleTransition(
			db, log, asSvc, "terminating", mux.Vars(r)["instance_id"])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"handling lifecycle transition failed: %s"}`, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"instance termination complete"}`)
	}
}
