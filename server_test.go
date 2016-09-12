package cyclist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
	srv := newServer("17321", "nz-mordor-1")
	assert.NotNil(t, srv)
	assert.NotNil(t, srv.r)
	assert.Equal(t, "17321", srv.port)
	assert.Equal(t, "nz-mordor-1", srv.awsRegion)
}

func TestServer_POST_sns_Confirmation(t *testing.T) {
	oldSg := sg
	oldDbPool := dbPool
	defer func() {
		sg = oldSg
		dbPool = oldDbPool
	}()

	sg = &testSNSGetter{}
	dbPool = &testRedisConnGetter{}

	srv := newServer("17321", "nz-mordor-1")
	ts := httptest.NewServer(srv.r)
	defer ts.Close()

	msg := &snsMessage{
		Type:     "SubscriptionConfirmation",
		Token:    "TOKEYTOKETOK",
		TopicARN: "arn:faf:af/af",
	}
	msgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", ts.URL),
		"application/json", msgBuf)
	assert.Nil(t, err)

	body := map[string]interface{}{}
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.Nil(t, err)

	assert.Equal(t, 200, res.StatusCode)
	assert.Contains(t, body, "ok")
}

func TestServer_POST_sns_Notification_UnknownLifecycleTransition(t *testing.T) {
	oldSg := sg
	oldDbPool := dbPool
	defer func() {
		sg = oldSg
		dbPool = oldDbPool
	}()

	sg = &testSNSGetter{}
	dbPool = &testRedisConnGetter{}

	srv := newServer("17321", "nz-mordor-1")
	ts := httptest.NewServer(srv.r)
	defer ts.Close()

	msgMsg := &lifecycleAction{
		LifecycleTransition: "autoscaling:PIZZA_PARTY",
	}
	msgMsgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)

	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}

	msgBuf := &bytes.Buffer{}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", ts.URL),
		"application/json", msgBuf)
	assert.Nil(t, err)

	body := map[string]interface{}{}
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.Nil(t, err)

	assert.Equal(t, 400, res.StatusCode)
	assert.Contains(t, body, "error")
	assert.Regexp(t, "unknown lifecycle transition.+", body["error"])
}

func TestServer_POST_sns_Notification_TestEvent(t *testing.T) {
	oldSg := sg
	oldDbPool := dbPool
	defer func() {
		sg = oldSg
		dbPool = oldDbPool
	}()

	sg = &testSNSGetter{}
	dbPool = &testRedisConnGetter{}

	srv := newServer("17321", "nz-mordor-1")
	ts := httptest.NewServer(srv.r)
	defer ts.Close()

	msgMsg := &lifecycleAction{
		Event: "autoscaling:TEST_NOTIFICATION",
	}
	msgMsgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)

	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}

	msgBuf := &bytes.Buffer{}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", ts.URL),
		"application/json", msgBuf)
	assert.Nil(t, err)

	body := map[string]interface{}{}
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.Nil(t, err)

	assert.Equal(t, 202, res.StatusCode)
	assert.Contains(t, body, "ok")
}

func TestServer_POST_sns_Notification_InstanceLaunchingLifecycleTransition(t *testing.T) {
	oldSg := sg
	oldDbPool := dbPool
	defer func() {
		sg = oldSg
		dbPool = oldDbPool
	}()

	sg = &testSNSGetter{}
	dbPool = &testRedisConnGetter{}

	srv := newServer("17321", "nz-mordor-1")
	ts := httptest.NewServer(srv.r)
	defer ts.Close()

	msgMsg := &lifecycleAction{
		LifecycleTransition: "autoscaling:EC2_INSTANCE_LAUNCHING",
	}
	msgMsgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)

	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}

	msgBuf := &bytes.Buffer{}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(t, err)

	res, err := http.Post(fmt.Sprintf("%s/sns", ts.URL),
		"application/json", msgBuf)
	assert.Nil(t, err)

	body := map[string]interface{}{}
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.Nil(t, err)

	assert.Equal(t, 400, res.StatusCode)
	assert.Contains(t, body, "error")
	assert.Regexp(t, "missing required fields in lifecycle action:.+", body["error"])
}

func TestServer_POST_sns_Notification_InstanceTerminatingLifecycleTransition(t *testing.T) {
	oldSg := sg
	oldDbPool := dbPool
	defer func() {
		sg = oldSg
		dbPool = oldDbPool
	}()

	sg = &testSNSGetter{}
	trcg := &testRedisConnGetter{}
	_ = trcg.Get()
	conn := trcg.Conn
	dbPool = trcg

	srv := newServer("17321", "nz-mordor-1")
	ts := httptest.NewServer(srv.r)
	defer ts.Close()

	msgMsg := &lifecycleAction{
		LifecycleTransition: "autoscaling:EC2_INSTANCE_TERMINATING",
		EC2InstanceID:       "i-fafafaf",
	}
	msgMsgBuf := &bytes.Buffer{}
	err := json.NewEncoder(msgMsgBuf).Encode(msgMsg)

	msg := &snsMessage{
		Type:    "Notification",
		Message: msgMsgBuf.String(),
	}

	msgBuf := &bytes.Buffer{}
	err = json.NewEncoder(msgBuf).Encode(msg)
	assert.Nil(t, err)

	conn.Command("HMSET", "cyclist:instance:i-fafafaf", "expected_state", "down").Expect("OK!")

	res, err := http.Post(fmt.Sprintf("%s/sns", ts.URL),
		"application/json", msgBuf)
	assert.Nil(t, err)

	body := map[string]interface{}{}
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.Nil(t, err)

	assert.Equal(t, 400, res.StatusCode)
	assert.Contains(t, body, "error")
	assert.Regexp(t, "missing required fields in lifecycle action:.+", body["error"])
}
