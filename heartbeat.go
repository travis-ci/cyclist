package cyclist

import (
	"fmt"
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
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"error": %q}`, err.Error())
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"state":%q}`, state)
	}
}
