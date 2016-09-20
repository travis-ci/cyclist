package cyclist

import (
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/pborman/uuid"
)

type tokenGenerator interface {
	GenerateToken() string
}

type uuidTokenGenerator struct{}

func (utg *uuidTokenGenerator) GenerateToken() string {
	return uuid.NewRandom().String()
}

func newTokensHandlerFunc(db repo, log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		instanceID, ok := mux.Vars(req)["instance_id"]
		if !ok {
			jsonRespond(w, http.StatusBadRequest, &jsonErr{Err: errNoInstanceID})
			return
		}

		instTok, err := db.fetchTempInstanceToken(instanceID)
		if err != nil {
			jsonRespond(w, http.StatusNotFound, &jsonErr{
				Err: fmt.Errorf("no token for instance '%s'", instanceID),
			})
			return
		}

		err = db.storeInstanceToken(instanceID, instTok)
		if err != nil {
			jsonRespond(w, http.StatusInternalServerError, &jsonErr{Err: err})
			return
		}

		jsonRespond(w, http.StatusOK, &jsonInstanceToken{Token: instTok})
	}
}

type jsonInstanceToken struct {
	Token string `json:"token"`
}
