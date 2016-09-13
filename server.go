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

	db repo
	l  *logrus.Logger
	a  autoscalingiface.AutoScalingAPI
	s  snsiface.SNSAPI
	r  *mux.Router
}

func (srv *server) ohai(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text-plain;charset=utf-8")
	fmt.Fprintf(w, "ohai™\n")
}

func (srv *server) Serve() error {
	if srv.r == nil {
		srv.setupRouter()
	}

	srv.l.WithField("port", srv.port).Info("serving")

	err := http.ListenAndServe(srv.port, negroni.New(
		negroni.NewRecovery(),
		negronilogrus.NewMiddleware(),
		negroni.Wrap(srv.r),
	))

	if err != nil {
		srv.l.WithField("err", err).Error("failed to serve")
	}
	return err
}

func (srv *server) setupRouter() {
	srv.r = mux.NewRouter()
	srv.r.HandleFunc(`/sns`, newSnsHandlerFunc(srv.db, srv.l, srv.s)).Methods("POST")

	srv.r.HandleFunc(`/heartbeats/{instance_id}`,
		newInstanceHeartbeatHandlerFunc(srv.db, srv.l)).Methods("GET")

	srv.r.HandleFunc(`/launches/{instance_id}`,
		newInstanceLaunchHandlerFunc(srv.db, srv.l, srv.a)).Methods("POST")

	srv.r.HandleFunc(`/terminations/{instance_id}`,
		newInstanceTerminationHandlerFunc(srv.db, srv.l, srv.a)).Methods("POST")

	srv.r.HandleFunc(`/`, srv.ohai).Methods("GET", "HEAD")
}
