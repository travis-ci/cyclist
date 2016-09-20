package cyclist

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/gorilla/mux"
	negronilogrus "github.com/meatballhat/negroni-logrus"
	"github.com/urfave/negroni"
)

var (
	errUnauthorized = errors.New("unauthorized")
	errForbidden    = errors.New("forbidden")
	errNoInstanceID = errors.New("no instance id found")
)

type server struct {
	port       string
	authTokens []string

	db     repo
	log    logrus.FieldLogger
	asSvc  autoscalingiface.AutoScalingAPI
	snsSvc snsiface.SNSAPI
	tokGen tokenGenerator
	router *mux.Router

	snsVerify bool
}

func (srv *server) ohai(w http.ResponseWriter, req *http.Request) {
	jsonRespond(w, http.StatusOK, &jsonMsg{Message: "ohai™"})
}

func (srv *server) Serve() error {
	if srv.authTokens == nil {
		srv.authTokens = []string{}
	}

	if srv.router == nil {
		srv.setupRouter()
	}

	srv.log.WithField("port", srv.port).Info("serving")

	err := http.ListenAndServe(srv.port, negroni.New(
		negroni.NewRecovery(),
		negronilogrus.NewMiddleware(),
		negroni.Wrap(srv.router),
	))

	if err != nil {
		srv.log.WithField("err", err).Error("failed to serve")
	}
	return err
}

func (srv *server) setupRouter() {
	srv.router = mux.NewRouter()
	srv.router.HandleFunc(`/sns`,
		newSNSHandlerFunc(srv.db, srv.log, srv.snsSvc, srv.snsVerify, srv.tokGen)).Methods("POST")

	srv.router.Handle(`/tokens/{instance_id}`,
		srv.authd(newTokensHandlerFunc(srv.db, srv.log))).Methods("GET")

	srv.router.Handle(`/heartbeats/{instance_id}`,
		srv.instAuthd(newHeartbeatHandlerFunc(srv.db, srv.log))).Methods("GET")

	srv.router.Handle(`/launches/{instance_id}`,
		srv.instAuthd(newLifecycleHandlerFunc("launch", srv.db, srv.log, srv.asSvc))).Methods("POST")

	srv.router.Handle(`/terminations/{instance_id}`,
		srv.instAuthd(newLifecycleHandlerFunc("termination", srv.db, srv.log, srv.asSvc))).Methods("POST")

	srv.router.Handle(`/events/{instance_id}`,
		srv.instAuthd(newLifecycleEventsHandlerFunc(srv.db, srv.log))).Methods("GET")

	srv.router.HandleFunc(`/`, srv.ohai).Methods("GET", "HEAD")
}

func (srv *server) authd(f http.HandlerFunc) http.Handler {
	return negroni.New(negroni.HandlerFunc(srv.requireAuth), negroni.Wrap(http.HandlerFunc(f)))
}

func (srv *server) requireAuth(w http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if authHeader == "" {
		w.Header().Set("WWW-Authenticate", "token")
		jsonRespond(w, http.StatusUnauthorized, &jsonErr{Err: errUnauthorized})
		return
	}

	for _, tok := range srv.authTokens {
		if subtle.ConstantTimeCompare([]byte(authHeader), []byte(fmt.Sprintf("token %s", tok))) == 1 {
			next(w, req)
			return
		}
	}

	jsonRespond(w, http.StatusForbidden, &jsonErr{Err: errForbidden})
}

func (srv *server) instAuthd(f http.HandlerFunc) http.Handler {
	return negroni.New(negroni.HandlerFunc(srv.requireInstAuth), negroni.Wrap(http.HandlerFunc(f)))
}

func (srv *server) requireInstAuth(w http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if authHeader == "" {
		w.Header().Set("WWW-Authenticate", "token")
		jsonRespond(w, http.StatusUnauthorized, &jsonErr{Err: errUnauthorized})
		return
	}

	instanceID, ok := mux.Vars(req)["instance_id"]
	if !ok {
		jsonRespond(w, http.StatusBadRequest, &jsonErr{Err: errNoInstanceID})
		return
	}

	instTok, err := srv.db.fetchInstanceToken(instanceID)
	if err != nil {
		jsonRespond(w, http.StatusForbidden, &jsonErr{Err: errForbidden})
		return
	}

	if subtle.ConstantTimeCompare([]byte(authHeader), []byte(fmt.Sprintf("token %s", instTok))) == 1 {
		next(w, req)
		return
	}

	jsonRespond(w, http.StatusForbidden, &jsonErr{Err: errForbidden})
}

func txtRespond(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, "%v", data)
}

func jsonRespond(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logrus.WithField("err", err).Error("failed to marshal data to json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"something awful happened, but it's a secret™"}`)
		return
	}
	w.WriteHeader(status)
	fmt.Fprintf(w, string(jsonBytes))
}

type jsonErr struct {
	Err error
}

func (je *jsonErr) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"error":%q}`, je.Err.Error())), nil
}

type jsonMsg struct {
	Message string `json:"message"`
}
