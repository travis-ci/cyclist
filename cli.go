package cyclist

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"

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
			&cli.UintFlag{
				Name:    "event-ttl",
				Value:   uint(60 * 60 * 48),
				Usage:   "duration in seconds since last update that instance lifecycle event data will be kept",
				EnvVars: []string{"CYCLIST_EVENT_TTL", "EVENT_TTL"},
			},
			&cli.UintFlag{
				Name:    "token-ttl",
				Value:   uint(60 * 5),
				Usage:   "duration in seconds since last access that instance token will be kept",
				EnvVars: []string{"CYCLIST_TOKEN_TTL", "TOKEN_TTL"},
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
						Value:   ":9753",
						Usage:   "the `PORT` (or full address) on which to serve",
						Aliases: []string{"p"},
						EnvVars: []string{"CYCLIST_PORT", "PORT"},
					},
					&cli.StringFlag{
						Name:    "auth-tokens",
						Usage:   "comma-delimited strings used for token auth of mutative requests",
						Aliases: []string{"T"},
						EnvVars: []string{"CYCLIST_AUTH_TOKENS", "AUTH_TOKENS"},
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
						EnvVars: []string{"CYCLIST_QUEUE_URL", "QUEUE_URL"},
					},
					&cli.IntFlag{
						Name:    "concurrency",
						Value:   2,
						Usage:   "the number of concurrent SQS workers to run",
						Aliases: []string{"C"},
						EnvVars: []string{"CYCLIST_CONCURRENCY", "CONCURRENCY"},
					},
				},
				Action: runSqs,
			},
		},
	}
}

func runServe(ctx *cli.Context) error {
	srv, err := runServeSetup(ctx)
	if err != nil {
		return err
	}
	return srv.Serve()
}

func runServeSetup(ctx *cli.Context) (*server, error) {
	port := ctx.String("port")
	if !strings.Contains(port, ":") {
		port = fmt.Sprintf(":%s", port)
	}

	log := buildLog(ctx.Bool("debug"))
	db := &redisRepo{
		cg:  buildRedisPool(ctx.String("redis-url")),
		log: log,

		instEventTTL: ctx.Uint("event-ttl"),
		instTokTTL:   ctx.Uint("token-ttl"),
	}

	snsSvc := sns.New(session.New(), &aws.Config{
		Region: aws.String(ctx.String("aws-region")),
	})
	asSvc := autoscaling.New(session.New(), &aws.Config{
		Region: aws.String(ctx.String("aws-region")),
	})

	authTokens := strings.Split(ctx.String("auth-tokens"), ",")
	for i, tok := range authTokens {
		authTokens[i] = strings.TrimSpace(tok)
	}

	return &server{
		port:       port,
		authTokens: authTokens,

		db:     db,
		log:    log,
		asSvc:  asSvc,
		snsSvc: snsSvc,
		tokGen: &uuidTokenGenerator{},

		snsVerify: true,
	}, nil
}

func runSqs(ctx *cli.Context) error {
	sh, cntx, err := runSqsSetup(ctx)
	if err != nil {
		return err
	}

	return sh.Run(cntx)
}

func runSqsSetup(ctx *cli.Context) (*sqsHandler, context.Context, error) {
	sqsQueueURL := ctx.String("queue-url")
	if sqsQueueURL == "" {
		return nil, nil, errors.New("missing SQS queue URL")
	}

	log := buildLog(ctx.Bool("debug"))
	db := &redisRepo{
		cg:  buildRedisPool(ctx.String("redis-url")),
		log: log,

		instEventTTL: ctx.Uint("event-ttl"),
		instTokTTL:   ctx.Uint("token-ttl"),
	}

	sqsSvc := sqs.New(session.New())
	snsSvc := sns.New(session.New(), &aws.Config{
		Region: aws.String(ctx.String("aws-region")),
	})
	asSvc := autoscaling.New(session.New(), &aws.Config{
		Region: aws.String(ctx.String("aws-region")),
	})

	cntx, cancel := context.WithCancel(context.Background())
	go runSignalHandler(cancel)

	return &sqsHandler{
		queueURL:    sqsQueueURL,
		concurrency: ctx.Int("concurrency"),

		db:     db,
		log:    log,
		asSvc:  asSvc,
		snsSvc: snsSvc,
		sqsSvc: sqsSvc,
	}, cntx, nil
}

func buildLog(debug bool) logrus.FieldLogger {
	log := logrus.New()
	if debug {
		log.Level = logrus.DebugLevel
	}
	log.WithField("level", log.Level).Debug("using log level")
	return log
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
