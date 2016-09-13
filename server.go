package cyclist

import (
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
	w.Header().Set("Content-Type", "text-plain;charset=utf-8")
	fmt.Fprintf(w, "ohai™\n")
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
		newInstanceLaunchHandlerFunc(srv.db, srv.log, srv.asSvc)).Methods("POST")

	srv.router.HandleFunc(`/terminations/{instance_id}`,
		newInstanceTerminationHandlerFunc(srv.db, srv.log, srv.asSvc)).Methods("POST")

	srv.router.HandleFunc(`/`, srv.ohai).Methods("GET", "HEAD")
}
