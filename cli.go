package cyclist

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"

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
				EnvVars: []string{"CYCLIST_AWS_REGION", "AWS_REGION"},
			},
			&cli.StringFlag{
				Name:    "redis-url",
				Value:   "redis://localhost:6379/0",
				Usage:   "the `REDIS_URL` used for cruddy fun",
				Aliases: []string{"R"},
				EnvVars: []string{"CYCLIST_REDIS_URL", "REDIS_URL"},
			},
			&cli.BoolFlag{
				Name:    "debug",
				Value:   false,
				Usage:   "set log level to debug",
				Aliases: []string{"D"},
				EnvVars: []string{"CYCLIST_DEBUG", "DEBUG"},
			},
		},
		Commands: []*cli.Command{
			{
				Name: "serve",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Value:   "127.0.0.1:9753",
						Usage:   "the `PORT` (or full address) on which to serve",
						Aliases: []string{"p"},
						EnvVars: []string{"CYCLIST_SERVER_PORT", "PORT"},
					},
				},
				Action: runServe,
			},
			{
				Name: "sqs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "queue-url",
						Usage:   "the `QUEUE_URL` from which to receive messages",
						Aliases: []string{"U"},
						EnvVars: []string{"CYCLIST_SQS_QUEUE_URL", "QUEUE_URL"},
					},
					&cli.IntFlag{
						Name:    "concurrency",
						Value:   runtime.NumCPU() * 2,
						Usage:   "the number of concurrent SQS workers to run",
						Aliases: []string{"C"},
						EnvVars: []string{"CYCLIST_SQS_CONCURRENCY", "CONCURRENCY"},
					},
				},
				Action: runSqs,
			},
		},
	}
}

func runServe(ctx *cli.Context) error {
	err := setupSharedResources(ctx.Bool("debug"), ctx.String("redis-url"))
	if err != nil {
		return err
	}

	port := ctx.String("port")
	if !strings.Contains(port, ":") {
		port = fmt.Sprintf("127.0.0.1:%s", port)
	}

	return newServer(port, ctx.String("aws-region")).Serve()
}

func runSqs(ctx *cli.Context) error {
	sqsQueueURL := ctx.String("queue-url")
	if sqsQueueURL == "" {
		return errors.New("missing SQS queue URL")
	}

	err := setupSharedResources(ctx.Bool("debug"), ctx.String("redis-url"))
	if err != nil {
		return err
	}

	cntx, cancel := context.WithCancel(context.Background())
	go runSignalHandler(cancel)
	return newSqsHandler(sqsQueueURL, ctx.Int("concurrency")).Run(cntx)
}

func setupSharedResources(debug bool, redisURL string) error {
	var err error

	log = logrus.New()
	if debug {
		log.Level = logrus.DebugLevel
	}
	log.WithField("level", log.Level).Debug("using log level")

	dbPool, err = buildRedisPool(redisURL)
	if err != nil {
		return err
	}
	log.WithField("db_pool", dbPool).Debug("built database pool")

	err = func() error {
		rc := dbPool.Get()
		defer rc.Close()
		_, err = rc.Do("PING")
		return err
	}()

	return err
}

func runSignalHandler(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	for {
		select {
		case <-sigChan:
			cancel()
			os.Exit(0)
		}
	}
}
