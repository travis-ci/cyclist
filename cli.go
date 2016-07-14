package cyclist

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/meatballhat/negroni-logrus"
	"github.com/urfave/negroni"
	"gopkg.in/urfave/cli.v2"
)

// RunCLI runs the cli oh wow!
func RunCLI(argv []string) {
	app := &cli.App{
		Usage:     "AWS ASG LIFECYCLE THING",
		Version:   VersionString,
		Copyright: CopyrightString,
		Commands: []*cli.Command{
			{
				Name: "serve",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Value:   "[::1]:9753",
						Usage:   "the `PORT` (or full address) on which to serve",
						Aliases: []string{"p"},
						EnvVars: []string{"TRAVIS_CYCLIST_PORT", "PORT"},
					},
				},
				Action: runServe,
			},
		},
	}
	app.Run(argv)
}

func runServe(ctx *cli.Context) error {
	port := ctx.String("port")
	if !strings.Contains(port, ":") {
		port = fmt.Sprintf("[::1]:%s", port)
	}
	return newServer(port).Serve()
}

type server struct {
	port string
	r    *mux.Router
}

func newServer(port string) *server {
	srv := &server{port: port}
	srv.setupRouter()
	return srv
}

func (srv *server) ohai(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "ohai\n")
}

func (srv *server) Serve() error {
	logrus.WithField("port", srv.port).Info("serving")

	return http.ListenAndServe(srv.port, negroni.New(
		negroni.NewRecovery(),
		negronilogrus.NewMiddleware(),
		negroni.Wrap(srv.r),
	))
}

func (srv *server) setupRouter() {
	srv.r = mux.NewRouter()
	srv.r.HandleFunc("/sns", snsHandler).Methods("POST")
	srv.r.HandleFunc("/", srv.ohai).Methods("GET", "HEAD")
}
