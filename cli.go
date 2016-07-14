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

// NewCLI makes the cli oh wow!
func NewCLI() *cli.App {
	return &cli.App{
		Usage:     "AWS ASG LIFECYCLE THING",
		Version:   VersionString,
		Copyright: CopyrightString,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "aws-region",
				Aliases: []string{"r"},
				Value:   "us-east-1",
				Usage:   "AWS region to use for the stuff",
				EnvVars: []string{"TRAVIS_CYCLIST_AWS_REGION", "AWS_REGION"},
			},
		},
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
}

func runServe(ctx *cli.Context) error {
	port := ctx.String("port")
	if !strings.Contains(port, ":") {
		port = fmt.Sprintf("[::1]:%s", port)
	}
	return newServer(port, ctx.String("aws-region")).Serve()
}

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
	logrus.WithField("port", srv.port).Info("serving")

	err := http.ListenAndServe(srv.port, negroni.New(
		negroni.NewRecovery(),
		negronilogrus.NewMiddleware(),
		negroni.Wrap(srv.r),
	))

	if err != nil {
		logrus.WithField("err", err).Error("failed to serve")
	}
	return err
}

func (srv *server) setupRouter() {
	srv.r = mux.NewRouter()
	srv.r.HandleFunc("/sns", newSnsHandlerFunc(srv.awsRegion)).Methods("POST")
	srv.r.HandleFunc("/", srv.ohai).Methods("GET", "HEAD")
}
