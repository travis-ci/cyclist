package cyclist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestServer() *server {
	srv := &server{
		port:       "17321",
		authTokens: []string{"mysteriously"},

		db:     newTestRepo(),
		log:    shushLog,
		asSvc:  newTestAutosScalingService(nil),
		snsSvc: newTestSNSService(nil),
	}
	srv.setupRouter()
	return srv
}

func TestServer_POST_sns_Confirmation(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
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
	assert.Contains(t, body, "message")
	assert.Equal(t, "subscription confirmed", body["message"])
}

func TestServer_POST_sns_Notification_UnknownLifecycleTransition(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
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
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
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
	assert.Contains(t, body, "message")
	assert.Equal(t, "notification handled", body["message"])
}

func TestServer_POST_sns_Notification_InstanceLaunchingLifecycleTransition(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
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
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
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

func TestServer_GET_heartbeats(t *testing.T) {
	srv := newTestServer()
	_ = srv.db.setInstanceState("i-fafafaf", "up")
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	res, err := http.Get(fmt.Sprintf("%s/heartbeats/i-fafafaf", ts.URL))
	assert.Nil(t, err)

	body := map[string]interface{}{}
	err = json.NewDecoder(res.Body).Decode(&body)
	assert.Nil(t, err)

	assert.Equal(t, 200, res.StatusCode)
	assert.Contains(t, body, "state")
	assert.Equal(t, "up", body["state"])
}

func TestServer_POST_launches(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	err := srv.db.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "launching",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "whimsical-mime-headphone",
		LifecycleHookName:    "greased-banana-net",
	})
	assert.Nil(t, err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/launches/i-fafafaf", ts.URL), &bytes.Buffer{})
	assert.Nil(t, err)
	assert.NotNil(t, req)
	req.Header.Set("Authorization", "token mysteriously")

	res, err := (&http.Client{}).Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, res)

	bodyBytes, err := ioutil.ReadAll(res.Body)
	assert.Nil(t, err)
	body := map[string]interface{}{}
	err = json.Unmarshal(bodyBytes, &body)
	assert.Nil(t, err)

	assert.Equal(t, 200, res.StatusCode)
	assert.Contains(t, body, "message")
	assert.Equal(t, "instance launch complete", body["message"])
}

func TestServer_POST_launches_WithoutAuthorizationHeader(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	err := srv.db.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "launching",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "whimsical-mime-headphone",
		LifecycleHookName:    "greased-banana-net",
	})
	assert.Nil(t, err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/launches/i-fafafaf", ts.URL), &bytes.Buffer{})
	assert.Nil(t, err)
	assert.NotNil(t, req)

	res, err := (&http.Client{}).Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, res)

	bodyBytes, err := ioutil.ReadAll(res.Body)
	assert.Nil(t, err)
	body := map[string]interface{}{}
	err = json.Unmarshal(bodyBytes, &body)
	assert.Nil(t, err)

	assert.Equal(t, 401, res.StatusCode)
	assert.Contains(t, body, "error")
	assert.Equal(t, "unauthorized", body["error"])
}

func TestServer_POST_launches_WithInvalidAuthorization(t *testing.T) {
	srv := newTestServer()
	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	err := srv.db.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "launching",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "whimsical-mime-headphone",
		LifecycleHookName:    "greased-banana-net",
	})
	assert.Nil(t, err)

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/launches/i-fafafaf", ts.URL), &bytes.Buffer{})
	assert.Nil(t, err)
	assert.NotNil(t, req)
	req.Header.Set("Authorization", "token infallible")

	res, err := (&http.Client{}).Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, res)

	bodyBytes, err := ioutil.ReadAll(res.Body)
	assert.Nil(t, err)
	body := map[string]interface{}{}
	err = json.Unmarshal(bodyBytes, &body)
	assert.Nil(t, err)

	assert.Equal(t, 403, res.StatusCode)
	assert.Contains(t, body, "error")
	assert.Equal(t, "forbidden", body["error"])
}
