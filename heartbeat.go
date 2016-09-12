package cyclist

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func newInstanceHeartbeatHandlerFunc(awsRegion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		instanceID := vars["instance_id"]

		log.WithField("instance_id", instanceID).Debug("fetching state")
		state, err := fetchInstanceState(dbPool.Get(), instanceID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"error": %q}`, err.Error())
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"state":%q}`, state)
	}
}
