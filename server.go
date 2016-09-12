package cyclist

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	negronilogrus "github.com/meatballhat/negroni-logrus"
	"github.com/urfave/negroni"
)

type server struct {
	port, awsRegion string

	r *mux.Router
}

func newServer(port, awsRegion string) *server {
	srv := &server{port: port, awsRegion: awsRegion}
	srv.setupRouter()
	return srv
}

func (srv *server) ohai(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "ohai\n")
}

func (srv *server) Serve() error {
	log.WithField("port", srv.port).Info("serving")

	err := http.ListenAndServe(srv.port, negroni.New(
		negroni.NewRecovery(),
		negronilogrus.NewMiddleware(),
		negroni.Wrap(srv.r),
	))

	if err != nil {
		log.WithField("err", err).Error("failed to serve")
	}
	return err
}

func (srv *server) setupRouter() {
	srv.r = mux.NewRouter()
	srv.r.HandleFunc(`/sns`, newSnsHandlerFunc(srv.awsRegion)).Methods("POST")
	srv.r.HandleFunc(`/status/{instance_id}`, newStatusGetHandlerFunc(srv.awsRegion)).Methods("GET")
	srv.r.HandleFunc(`/status/{instance_id}`, newStatusPutHandlerFunc(srv.awsRegion)).Methods("PUT")
	srv.r.HandleFunc(`/`, srv.ohai).Methods("GET", "HEAD")
}
