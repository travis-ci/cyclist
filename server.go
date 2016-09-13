package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/gorilla/mux"
	negronilogrus "github.com/meatballhat/negroni-logrus"
	"github.com/urfave/negroni"
)

type server struct {
	port string

	db     repo
	log    *logrus.Logger
	asSvc  autoscalingiface.AutoScalingAPI
	snsSvc snsiface.SNSAPI
	router *mux.Router
}

func (srv *server) ohai(w http.ResponseWriter, req *http.Request) {
	jsonRespond(w, http.StatusOK, &jsonMsg{Message: "ohai™"})
}

func (srv *server) Serve() error {
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
	srv.router.HandleFunc(`/sns`, newSnsHandlerFunc(srv.db, srv.log, srv.snsSvc)).Methods("POST")

	srv.router.HandleFunc(`/heartbeats/{instance_id}`,
		newInstanceHeartbeatHandlerFunc(srv.db, srv.log)).Methods("GET")

	srv.router.HandleFunc(`/launches/{instance_id}`,
		newInstanceLifecycleHandlerFunc("launch", srv.db, srv.log, srv.asSvc)).Methods("POST")

	srv.router.HandleFunc(`/terminations/{instance_id}`,
		newInstanceLifecycleHandlerFunc("termination", srv.db, srv.log, srv.asSvc)).Methods("POST")

	srv.router.HandleFunc(`/`, srv.ohai).Methods("GET", "HEAD")
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
