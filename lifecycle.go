package cyclist

import (
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/gorilla/mux"
)

func handleLifecycleTransition(awsRegion string, t *lifecycleTransition) error {
	rc := dbPool.Get()
	action, err := fetchInstanceLifecycleAction(rc, t.Transition, t.InstanceID)
	if err != nil {
		return err
	}

	if action == nil {
		return fmt.Errorf("no lifecycle transition '%s' for instance '%s'", t.Transition, t.InstanceID)
	}

	svc := ag.Get(awsRegion)

	input := &autoscaling.CompleteLifecycleActionInput{
		AutoScalingGroupName:  aws.String(action.AutoScalingGroupName),
		InstanceId:            aws.String(t.InstanceID),
		LifecycleActionResult: aws.String("CONTINUE"),
		LifecycleActionToken:  aws.String(action.LifecycleActionToken),
		LifecycleHookName:     aws.String(action.LifecycleHookName),
	}
	_, err = svc.CompleteLifecycleAction(input)
	if err != nil {

		return err
	}

	err = wipeInstanceLifecycleAction(rc, t.Transition, t.InstanceID)
	if err != nil {
		log.WithField("err", err).Warn("failed to clean up lifecycle action bits")
	}

	return nil
}

func newInstanceLaunchHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := &lifecycleTransition{
			Transition: "launching",
			InstanceID: mux.Vars(r)["instance_id"],
		}

		w.Header().Set("Content-Type", "application/json")

		err := handleLifecycleTransition(awsRegion, t)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"handling lifecycle transition failed: %s"}`, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"instance launch complete"}`)
	}
}

func newInstanceTerminationHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := &lifecycleTransition{
			Transition: "terminating",
			InstanceID: mux.Vars(r)["instance_id"],
		}

		w.Header().Set("Content-Type", "application/json")

		err := handleLifecycleTransition(awsRegion, t)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"handling lifecycle transition failed: %s"}`, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"instance termination complete"}`)
	}
}
