package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/gorilla/mux"
	"github.com/pborman/uuid"
)

func handleLifecycleTransition(awsRegion string, t *lifecycleTransition) error {
	rc := dbPool.Get()
	action, err := fetchInstanceLifecycleAction(rc, t.Transition, t.InstanceID)
	if err != nil {
		return err
	}

	if action == nil {
		// TODO: ERR: NO ACTION FOUND
		return nil
	}

	svc := autoscaling.New(
		session.New(),
		&aws.Config{
			Region: aws.String(awsRegion),
		})

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

func newStatusGetHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// vars := mux.Vars(request)
		// instanceID := vars["instanceID"]

		// TODO: return status and expected_state
	}
}

func newStatusPutHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		instanceID := vars["instance_id"]

		t := &lifecycleTransition{}
		t.ID = uuid.NewUUID().String()
		t.InstanceID = instanceID

		err := json.NewDecoder(r.Body).Decode(t)
		if err != nil {
			fmt.Fprintf(w, "invalid json received: %s\n", err)
			return
		}

		err = handleLifecycleTransition(awsRegion, t)
		if err != nil {
			fmt.Fprintf(w, "handling lifecycle transition falied: %s\n", err)
			return
		}

		// TODO: json response?
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "status updated successfully\n")
	}
}
