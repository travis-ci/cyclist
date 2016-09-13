package cyclist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"gopkg.in/urfave/cli.v2"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestFullLifecycleManagementHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	app := NewCLI()
	for _, command := range app.Commands {
		if command.Name != "serve" {
			continue
		}
		command.Action = newFullLifecycleManagementHTTP(t).Run
	}

	app.Run([]string{"cyclist", "serve"})
}

type fullLifecycleManagementHTTP struct {
	bag map[string]string

	t   *testing.T
	srv *server
	db  repo
	ts  *httptest.Server
}

func newFullLifecycleManagementHTTP(t *testing.T) *fullLifecycleManagementHTTP {
	return &fullLifecycleManagementHTTP{
		bag: map[string]string{
			"instance_id": fmt.Sprintf("i-fa%d%d%d", rand.Int(), rand.Int(), rand.Int())[:10],
		},

		t: t,
	}
}

func (f *fullLifecycleManagementHTTP) Run(ctx *cli.Context) error {
	f.t.Logf("starting lifecycle test for instance %q", f.bag["instance_id"])
	f.t.Logf("initializing server")
	f.stepInitServer(ctx)
	defer f.ts.Close()

	steps := []struct {
		desc     string
		stepFunc func()
	}{
		{"handling SNS subscription confirmation", f.stepHandleSubscriptionConfirmation},
		{"handling SNS test notification", f.stepHandleTestNotification},
		{"handling SNS instance launching notification", f.stepHandleInstanceLaunchingNotification},
		{"handling instance launching confirmation", f.stepHandleInstanceLaunchingConfirmation},
		{"handling heartbeat while up", f.stepHandleHeartbeatWhileUp},
		{"handling SNS instance terminating notification", f.stepHandleInstanceTerminatingNotification},
		{"handling heartbeat while down", f.stepHandleHeartbeatWhileDown},
		{"handling instance terminating confirmation", f.stepHandleInstanceTerminatingConfirmation},
	}

	nSteps := len(steps)
	for i, step := range steps {
		f.t.Logf("step %d/%d: %s", i+1, nSteps, step.desc)
		step.stepFunc()
	}

	f.t.Logf("done with lifecycle test for instance %q", f.bag["instance_id"])
	return nil
}

func (f *fullLifecycleManagementHTTP) stepInitServer(ctx *cli.Context) {
	srv, err := runServeSetup(ctx)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, srv)

	srv.db = newTestRepo()
	assert.NotNil(f.t, srv.db)

	srv.asSvc = newTestAutosScalingService(f.autoScalingCallback)
	srv.snsSvc = newTestSNSService(f.snsCallback)
	if os.Getenv("DEBUG") != "1" {
		srv.log.Level = logrus.FatalLevel
	}

	f.srv = srv
	f.db = srv.db

	srv.setupRouter()
	f.ts = httptest.NewServer(srv.router)
}

func (f *fullLifecycleManagementHTTP) autoScalingCallback(req *request.Request) {
	if v, ok := req.Params.(*autoscaling.CompleteLifecycleActionInput); ok {
		assert.Equal(f.t, *v.InstanceId, f.bag["instance_id"])
		assert.Equal(f.t, *v.LifecycleActionResult, "CONTINUE")
		assert.Equal(f.t, *v.LifecycleActionToken, f.bag[fmt.Sprintf("instance_%s_token", f.bag["lifecycle_action"])])
		return
	}
	req.Error = errors.New("is not good")
}

func (f *fullLifecycleManagementHTTP) snsCallback(req *request.Request) {
	if v, ok := req.Params.(*sns.ConfirmSubscriptionInput); ok {
		assert.Equal(f.t, *v.Token, f.bag["sns_subscription_token"])
		return
	}
	req.Error = errors.New("nope nope nope")
}

func (f *fullLifecycleManagementHTTP) stepHandleSubscriptionConfirmation() {
	f.bag["sns_subscription_token"] = uuid.NewUUID().String()
	msg := &snsMessage{
		Type:     "SubscriptionConfirmation",
		Token:    f.bag["sns_subscription_token"],
		TopicARN: "arn:aws:sns:nz-isengard-1:999999999999:toaster-pastries",
	}
	msgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(f.t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", f.ts.URL), "application/json", msgBuf)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)
}

func (f *fullLifecycleManagementHTTP) stepHandleTestNotification() {
	msgMsgBuf := &bytes.Buffer{}
	msgMsg := &lifecycleAction{
		Event: "autoscaling:TEST_NOTIFICATION",
	}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)
	assert.Nil(f.t, err)

	msgBuf := &bytes.Buffer{}
	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(f.t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", f.ts.URL), "application/json", msgBuf)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)
	assert.Equal(f.t, 202, res.StatusCode)
}

func (f *fullLifecycleManagementHTTP) stepHandleInstanceLaunchingNotification() {
	f.bag["lifecycle_action"] = "launching"
	f.bag["instance_launching_token"] = uuid.NewUUID().String()
	msgMsgBuf := &bytes.Buffer{}
	msgMsg := &lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LAUNCHING",
		EC2InstanceID:        f.bag["instance_id"],
		LifecycleActionToken: f.bag["instance_launching_token"],
		AutoScalingGroupName: "cyclist-integration-test-asg",
		LifecycleHookName:    "cyclist-integration-test-lch-launching",
	}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)
	assert.Nil(f.t, err)

	msgBuf := &bytes.Buffer{}
	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(f.t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", f.ts.URL), "application/json", msgBuf)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)
	assert.Equal(f.t, 200, res.StatusCode)

	la, err := f.db.fetchInstanceLifecycleAction("launching", f.bag["instance_id"])
	assert.Nil(f.t, err)
	assert.NotNil(f.t, la)
}

func (f *fullLifecycleManagementHTTP) stepHandleInstanceLaunchingConfirmation() {
	res, err := http.Post(fmt.Sprintf("%s/launches/%s", f.ts.URL, f.bag["instance_id"]),
		"application/octet-stream", &bytes.Buffer{})
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"message": "instance launch complete"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	la, err := f.db.fetchInstanceLifecycleAction("launching", f.bag["instance_id"])
	assert.Nil(f.t, la)
	assert.NotNil(f.t, err)

	state, err := f.db.fetchInstanceState(f.bag["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "up", state)
}

func (f *fullLifecycleManagementHTTP) stepHandleHeartbeatWhileUp() {
	res, err := http.Get(fmt.Sprintf("%s/heartbeats/%s", f.ts.URL, f.bag["instance_id"]))
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"state": "up"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	state, err := f.db.fetchInstanceState(f.bag["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "up", state)
}

func (f *fullLifecycleManagementHTTP) stepHandleInstanceTerminatingNotification() {
	f.bag["lifecycle_action"] = "terminating"
	f.bag["instance_terminating_token"] = uuid.NewUUID().String()
	msgMsgBuf := &bytes.Buffer{}
	msgMsg := &lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_TERMINATING",
		EC2InstanceID:        f.bag["instance_id"],
		LifecycleActionToken: f.bag["instance_terminating_token"],
		AutoScalingGroupName: "cyclist-integration-test-asg",
		LifecycleHookName:    "cyclist-integration-test-lch-terminating",
	}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)
	assert.Nil(f.t, err)

	msgBuf := &bytes.Buffer{}
	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(f.t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", f.ts.URL), "application/json", msgBuf)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)
	assert.Equal(f.t, 200, res.StatusCode)

	la, err := f.db.fetchInstanceLifecycleAction("terminating", f.bag["instance_id"])
	assert.Nil(f.t, err)
	assert.NotNil(f.t, la)

	state, err := f.db.fetchInstanceState(f.bag["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "down", state)
}

func (f *fullLifecycleManagementHTTP) stepHandleHeartbeatWhileDown() {
	res, err := http.Get(fmt.Sprintf("%s/heartbeats/%s", f.ts.URL, f.bag["instance_id"]))
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"state": "down"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	state, err := f.db.fetchInstanceState(f.bag["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "down", state)
}

func (f *fullLifecycleManagementHTTP) stepHandleInstanceTerminatingConfirmation() {
	res, err := http.Post(fmt.Sprintf("%s/terminations/%s", f.ts.URL, f.bag["instance_id"]),
		"application/octet-stream", &bytes.Buffer{})
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"message": "instance termination complete"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	la, err := f.db.fetchInstanceLifecycleAction("terminating", f.bag["instance_id"])
	assert.Nil(f.t, la)
	assert.NotNil(f.t, err)

	state, err := f.db.fetchInstanceState(f.bag["instance_id"])
	assert.NotNil(f.t, err)
	assert.Equal(f.t, "", state)

	assert.Len(f.t, f.db.(*testRepo).s, 0)
	assert.Len(f.t, f.db.(*testRepo).la, 0)
}

func TestFullLifecycleManagementSQS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// TODO: this thing here
}
