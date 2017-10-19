package cyclist

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func newHeartbeatHandlerFunc(db repo, log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		instanceID := vars["instance_id"]

		log.WithField("instance_id", instanceID).Debug("fetching state")
		state, err := db.fetchInstanceState(instanceID)
		if err != nil {
			jsonRespond(w, http.StatusNotFound, &jsonErr{Err: err})
			return
		}

		err = db.storeInstanceEvent(instanceID, "heartbeat")
		if err != nil {
			jsonRespond(w, http.StatusInternalServerError, &jsonErr{Err: err})
		}

		jsonRespond(w, http.StatusOK, &jsonInstanceState{State: state})
	}
}

type jsonInstanceState struct {
	State string `json:"state"`
}
