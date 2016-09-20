package cyclist

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/pborman/uuid"
)

var (
	ctypeJSONRegexp = regexp.MustCompile("^(application/json|text/javascript)$")
	ctypeTextRegexp = regexp.MustCompile("^text/plain$")
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
		ctypeText := false
		for _, accepts := range strings.Split(req.Header.Get("Accept"), ",") {
			accepts = strings.TrimSpace(strings.Split(accepts, ";")[0])
			if ctypeTextRegexp.MatchString(accepts) {
				ctypeText = true
				break
			}
			if ctypeJSONRegexp.MatchString(accepts) {
				break
			}
		}

		if !ok {
			if ctypeText {
				txtRespond(w, http.StatusBadRequest, errNoInstanceID)
				return
			}
			jsonRespond(w, http.StatusBadRequest, &jsonErr{Err: errNoInstanceID})
			return
		}

		instTok, err := db.fetchTempInstanceToken(instanceID)
		if err != nil {
			notFoundErr := fmt.Errorf("no token for instance '%s'", instanceID)
			if ctypeText {
				txtRespond(w, http.StatusNotFound, notFoundErr)
				return
			}
			jsonRespond(w, http.StatusNotFound, &jsonErr{Err: notFoundErr})
			return
		}

		err = db.storeInstanceToken(instanceID, instTok)
		if err != nil {
			if ctypeText {
				txtRespond(w, http.StatusInternalServerError, err)
				return
			}
			jsonRespond(w, http.StatusInternalServerError, &jsonErr{Err: err})
			return
		}

		if ctypeText {
			txtRespond(w, http.StatusOK, instTok)
			return
		}
		jsonRespond(w, http.StatusOK, &jsonInstanceToken{Token: instTok})
	}
}

type jsonInstanceToken struct {
	Token string `json:"token"`
}
