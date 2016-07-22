package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/gorilla/feeds"
	"github.com/gorilla/mux"
	"github.com/travis-ci/pudding/db"
)

// TODO: implement db with redis connection pool

// instanceLifecycleTransition is an event received from instances when launching and terminating
// Transition can be launching or terminating
type instanceLifecycleTransition struct {
	ID         string `json:"id,omitempty"`
	InstanceID string `json:"instance_id"`
	Transition string `json:"transition"`
}

func handleInstanceLifecycleTransition(t instanceLifecycleTransition) error {
	action, err := db.FetchInstanceLifecycleAction(rc, t.Transition, t.InstanceID)
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
		AutoScalingGroupName:  action.AutoScalingGroupName,
		InstanceId:            t.InstanceID,
		LifecycleActionResult: "CONTINUE",
		LifecycleActionToken:  action.LifecycleActionToken,
		LifecycleHookName:     action.LifecycleHookName,
	}
	_, err = svc.CompleteLifecycleAction(input)
	if err != nil {
		return err
	}

	err = db.WipeInstanceLifecycleAction(rc, t.Transition, t.InstanceID)
	if err != nil {
		log.WithField("err", err).Warn("failed to clean up lifecycle action bits")
	}

	return nil
}

func newStatusGetHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// vars := mux.Vars(request)
		// instanceID := vars["instanceID"]

		// TODO: return status and expected_state
	}
}

func newStatusPutHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(request)
		instanceID := vars["instanceID"]

		// TODO: remove feeds dependency
		t := &instanceLifecycleTransition{}
		t.ID = feeds.NewUUID().String()
		t.InstanceID = instanceID

		err := json.NewDecoder(req.Body).Decode(t)
		if err != nil {
			fmt.Fprintf(w, "invalid json received: %s\n", err)
			return
		}

		err = handleInstanceLifecycleTransition(t)
		if err != nil {
			fmt.Fprintf(w, "handling lifecycle transition falied: %s\n", err)
			return
		}

		// TODO: json response?
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "status updated successfully\n")
	}
}
