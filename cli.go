package cyclist

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/sns"

	"gopkg.in/urfave/cli.v2"
)

var (
	defaultLogOut io.Writer = os.Stdout
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
			&cli.DurationFlag{
				Name:    "event-ttl",
				Value:   48 * time.Hour,
				Usage:   "duration since last update that instance lifecycle event data will be kept",
				EnvVars: []string{"CYCLIST_EVENT_TTL", "EVENT_TTL"},
			},
			&cli.DurationFlag{
				Name:    "temp-token-ttl",
				Value:   5 * time.Minute,
				Usage:   "duration that instance temporary token will be kept",
				EnvVars: []string{"CYCLIST_TEMP_TOKEN_TTL", "TEMP_TOKEN_TTL"},
			},
			&cli.DurationFlag{
				Name:    "token-ttl",
				Value:   time.Hour,
				Usage:   "duration since last access that instance token will be kept",
				EnvVars: []string{"CYCLIST_TOKEN_TTL", "TOKEN_TTL"},
			},
			&cli.DurationFlag{
				Name:    "lifecycle-action-ttl",
				Value:   7 * 24 * time.Hour,
				Usage:   "duration that lifecycle actions records will be kept",
				EnvVars: []string{"CYCLIST_LIFECYCLE_ACTION_TTL", "LIFECYCLE_ACTION_TTL"},
			},
			&cli.BoolFlag{
				Name:    "detach",
				Value:   false,
				Usage:   "detach instances on termination",
				EnvVars: []string{"CYCLIST_DETACH", "DETACH"},
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
				Name: "set-down",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "instances",
						Aliases: []string{"i"},
						Usage:   "the `INSTANCES` for which the instance state will be set to \"down\"",
						EnvVars: []string{"CYCLIST_INSTANCES", "INSTANCES"},
					},
				},
				Action: runSetDown,
			},
			/* TODO: #5
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
			*/
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

func runSetDown(ctx *cli.Context) error {
	log := buildLog(ctx.Bool("debug"))
	db := setupDbFromCtxAndLog(ctx, log)

	for _, instanceID := range ctx.StringSlice("instances") {
		err := db.setInstanceState(instanceID, "down")
		if err != nil {
			return err
		}
		log.WithFields(logrus.Fields{
			"instance_id": instanceID,
			"state":       "down",
		}).Info("set")
	}

	return nil
}

func runServeSetup(ctx *cli.Context) (*server, error) {
	port := ctx.String("port")
	if !strings.Contains(port, ":") {
		port = fmt.Sprintf(":%s", port)
	}

	log := buildLog(ctx.Bool("debug"))
	db := setupDbFromCtxAndLog(ctx, log)

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
		detach:    ctx.Bool("detach"),
	}, nil
}

func setupDbFromCtxAndLog(ctx *cli.Context, log logrus.FieldLogger) repo {
	return &redisRepo{
		cg:  buildRedisPool(ctx.String("redis-url")),
		log: log,

		instEventTTL:           uint(ctx.Duration("event-ttl").Seconds()),
		instLifecycleActionTTL: uint(ctx.Duration("lifecycle-action-ttl").Seconds()),
		instTempTokTTL:         uint(ctx.Duration("temp-token-ttl").Seconds()),
		instTokTTL:             uint(ctx.Duration("token-ttl").Seconds()),
	}
}

/* TODO: #5
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

		instEventTTL:						uint(ctx.Duration("event-ttl").Seconds()),
		instLifecycleActionTTL: uint(ctx.Duration("lifecycle-action-ttl").Seconds()),
		instTempTokTTL:					uint(ctx.Duration("temp-token-ttl").Seconds()),
		instTokTTL:							uint(ctx.Duration("token-ttl").Seconds()),
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
*/

func buildLog(debug bool) logrus.FieldLogger {
	log := logrus.New()
	log.Out = defaultLogOut
	if debug {
		log.Level = logrus.DebugLevel
	}
	log.WithField("level", log.Level).Debug("using log level")
	return log
}
