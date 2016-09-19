package cyclist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	vars map[string]string

	t   *testing.T
	srv *server
	db  repo
	ts  *httptest.Server
}

func newFullLifecycleManagementHTTP(t *testing.T) *fullLifecycleManagementHTTP {
	return &fullLifecycleManagementHTTP{
		vars: map[string]string{
			"instance_id": fmt.Sprintf("i-fa%d%d%d", rand.Int(), rand.Int(), rand.Int())[:10],
		},

		t: t,
	}
}

func (f *fullLifecycleManagementHTTP) Run(ctx *cli.Context) error {
	f.t.Logf("starting lifecycle test for instance %q", f.vars["instance_id"])
	f.t.Logf("initializing server")
	f.stepInitServer(ctx)
	defer f.ts.Close()

	steps := []struct {
		desc     string
		stepFunc func()
	}{
		{"SNS subscription confirmation", f.stepSubscriptionConfirmation},
		{"SNS test notification", f.stepTestNotification},
		{"SNS instance launching notification", f.stepInstanceLaunchingNotification},
		{"instance launching confirmation", f.stepInstanceLaunchingConfirmation},
		{"heartbeat while up", f.stepHeartbeatWhileUp},
		{"SNS instance terminating notification", f.stepInstanceTerminatingNotification},
		{"heartbeat while down", f.stepHeartbeatWhileDown},
		{"instance terminating confirmation", f.stepInstanceTerminatingConfirmation},
	}

	nSteps := len(steps)
	for i, step := range steps {
		f.t.Logf("step %d/%d: %s", i+1, nSteps, step.desc)
		step.stepFunc()
	}

	f.t.Logf("done with lifecycle test for instance %q", f.vars["instance_id"])
	return nil
}

func (f *fullLifecycleManagementHTTP) authPost(path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s%s", f.ts.URL, path), body)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, req)
	req.Header.Set("Authorization", fmt.Sprintf("token %s", f.srv.authTokens[0]))

	client := &http.Client{}
	return client.Do(req)
}

func (f *fullLifecycleManagementHTTP) stepInitServer(ctx *cli.Context) {
	srv, err := runServeSetup(ctx)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, srv)

	srv.authTokens = []string{fmt.Sprintf("%d", rand.Int())}

	srv.db = newTestRepo()
	assert.NotNil(f.t, srv.db)

	srv.snsVerify = false
	srv.asSvc = newTestAutosScalingService(f.autoScalingCallback)
	srv.snsSvc = newTestSNSService(f.snsCallback)
	if os.Getenv("DEBUG") != "1" {
		srv.log.(*logrus.Logger).Level = logrus.FatalLevel
	}

	f.srv = srv
	f.db = srv.db

	srv.setupRouter()
	f.ts = httptest.NewServer(srv.router)
}

func (f *fullLifecycleManagementHTTP) autoScalingCallback(req *request.Request) {
	if v, ok := req.Params.(*autoscaling.CompleteLifecycleActionInput); ok {
		assert.Equal(f.t, *v.InstanceId, f.vars["instance_id"])
		assert.Equal(f.t, *v.LifecycleActionResult, "CONTINUE")
		assert.Equal(f.t, *v.LifecycleActionToken, f.vars[fmt.Sprintf("instance_%s_token", f.vars["lifecycle_action"])])
		f.vars[fmt.Sprintf("instance_%s_state", f.vars["lifecycle_action"])] = "completed"
		return
	}
	req.Error = errors.New("is not good")
}

func (f *fullLifecycleManagementHTTP) snsCallback(req *request.Request) {
	if v, ok := req.Params.(*sns.ConfirmSubscriptionInput); ok {
		assert.Equal(f.t, *v.Token, f.vars["sns_subscription_token"])
		f.vars["sns_subscription_state"] = "confirmed"
		return
	}
	req.Error = errors.New("nope nope nope")
}

func (f *fullLifecycleManagementHTTP) stepSubscriptionConfirmation() {
	f.vars["sns_subscription_state"] = "unconfirmed"
	f.vars["sns_subscription_token"] = uuid.NewUUID().String()
	msg := &snsMessage{
		Type:     "SubscriptionConfirmation",
		Token:    f.vars["sns_subscription_token"],
		TopicARN: "arn:aws:sns:nz-isengard-1:999999999999:toaster-pastries",
	}
	msgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(f.t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", f.ts.URL), "application/json", msgBuf)
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)
	assert.Equal(f.t, "confirmed", f.vars["sns_subscription_state"])
}

func (f *fullLifecycleManagementHTTP) stepTestNotification() {
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

func (f *fullLifecycleManagementHTTP) stepInstanceLaunchingNotification() {
	f.vars["lifecycle_action"] = "launching"
	f.vars["instance_launching_state"] = "pending"
	f.vars["instance_launching_token"] = uuid.NewUUID().String()
	msgMsgBuf := &bytes.Buffer{}
	msgMsg := &lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LAUNCHING",
		EC2InstanceID:        f.vars["instance_id"],
		LifecycleActionToken: f.vars["instance_launching_token"],
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

	la, err := f.db.fetchInstanceLifecycleAction("launching", f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.NotNil(f.t, la)
	assert.Equal(f.t, "pending", f.vars["instance_launching_state"])

	state, err := f.db.fetchInstanceState(f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "up", state)

	res, err = http.Get(fmt.Sprintf("%s/events/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)

	evs := &jsonLifecycleEvents{Meta: map[string]string{}}
	err = json.NewDecoder(res.Body).Decode(evs)
	assert.Nil(f.t, err)
	assert.Len(f.t, evs.Data, 1)
}

func (f *fullLifecycleManagementHTTP) stepInstanceLaunchingConfirmation() {
	res, err := f.authPost(fmt.Sprintf("/launches/%s", f.vars["instance_id"]), &bytes.Buffer{})
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"message": "instance launch complete"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	la, err := f.db.fetchInstanceLifecycleAction("launching", f.vars["instance_id"])
	assert.Nil(f.t, la)
	assert.NotNil(f.t, err)

	state, err := f.db.fetchInstanceState(f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "up", state)
	assert.Equal(f.t, "completed", f.vars["instance_launching_state"])

	res, err = http.Get(fmt.Sprintf("%s/events/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)

	evs := &jsonLifecycleEvents{Meta: map[string]string{}}
	err = json.NewDecoder(res.Body).Decode(evs)
	assert.Nil(f.t, err)
	assert.Len(f.t, evs.Data, 2)
}

func (f *fullLifecycleManagementHTTP) stepHeartbeatWhileUp() {
	res, err := http.Get(fmt.Sprintf("%s/heartbeats/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"state": "up"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	state, err := f.db.fetchInstanceState(f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "up", state)

	res, err = http.Get(fmt.Sprintf("%s/events/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)

	evs := &jsonLifecycleEvents{Meta: map[string]string{}}
	err = json.NewDecoder(res.Body).Decode(evs)
	assert.Nil(f.t, err)
	assert.Len(f.t, evs.Data, 3)
}

func (f *fullLifecycleManagementHTTP) stepInstanceTerminatingNotification() {
	f.vars["lifecycle_action"] = "terminating"
	f.vars["instance_terminating_state"] = "pending"
	f.vars["instance_terminating_token"] = uuid.NewUUID().String()
	msgMsgBuf := &bytes.Buffer{}
	msgMsg := &lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_TERMINATING",
		EC2InstanceID:        f.vars["instance_id"],
		LifecycleActionToken: f.vars["instance_terminating_token"],
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

	la, err := f.db.fetchInstanceLifecycleAction("terminating", f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.NotNil(f.t, la)

	state, err := f.db.fetchInstanceState(f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "down", state)

	res, err = http.Get(fmt.Sprintf("%s/events/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)

	evs := &jsonLifecycleEvents{Meta: map[string]string{}}
	err = json.NewDecoder(res.Body).Decode(evs)
	assert.Nil(f.t, err)
	assert.Len(f.t, evs.Data, 4)
}

func (f *fullLifecycleManagementHTTP) stepHeartbeatWhileDown() {
	res, err := http.Get(fmt.Sprintf("%s/heartbeats/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"state": "down"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	state, err := f.db.fetchInstanceState(f.vars["instance_id"])
	assert.Nil(f.t, err)
	assert.Equal(f.t, "down", state)

	res, err = http.Get(fmt.Sprintf("%s/events/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)

	evs := &jsonLifecycleEvents{Meta: map[string]string{}}
	err = json.NewDecoder(res.Body).Decode(evs)
	assert.Nil(f.t, err)
	assert.Len(f.t, evs.Data, 4)
}

func (f *fullLifecycleManagementHTTP) stepInstanceTerminatingConfirmation() {
	res, err := f.authPost(fmt.Sprintf("/terminations/%s", f.vars["instance_id"]), &bytes.Buffer{})
	assert.Nil(f.t, err)
	assert.NotNil(f.t, res)

	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(f.t, err)
	assert.JSONEq(f.t, `{"message": "instance termination complete"}`, string(body))
	assert.Equal(f.t, 200, res.StatusCode)

	la, err := f.db.fetchInstanceLifecycleAction("terminating", f.vars["instance_id"])
	assert.Nil(f.t, la)
	assert.NotNil(f.t, err)

	state, err := f.db.fetchInstanceState(f.vars["instance_id"])
	assert.NotNil(f.t, err)
	assert.Equal(f.t, "", state)
	assert.Equal(f.t, "completed", f.vars["instance_terminating_state"])

	assert.Len(f.t, f.db.(*testRepo).s, 0)
	assert.Len(f.t, f.db.(*testRepo).la, 0)

	res, err = http.Get(fmt.Sprintf("%s/events/%s", f.ts.URL, f.vars["instance_id"]))
	assert.Nil(f.t, err)

	evs := &jsonLifecycleEvents{Meta: map[string]string{}}
	err = json.NewDecoder(res.Body).Decode(evs)
	assert.Nil(f.t, err)
	assert.Len(f.t, evs.Data, 5)
}

func TestFullLifecycleManagementSQS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	// TODO: this thing here
}
