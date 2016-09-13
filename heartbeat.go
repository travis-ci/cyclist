package cyclist

import (
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
)

func newInstanceHeartbeatHandlerFunc(db repo, log *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		instanceID := vars["instance_id"]

		log.WithField("instance_id", instanceID).Debug("fetching state")
		state, err := db.fetchInstanceState(instanceID)
		if err != nil {
			jsonRespond(w, http.StatusNotFound, &jsonErr{Err: err})
			return
		}

		jsonRespond(w, http.StatusOK, &jsonInstanceState{State: state})
	}
}

type jsonInstanceState struct {
	State string `json:"state"`
}
