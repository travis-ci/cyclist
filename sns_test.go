package cyclist

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/stretchr/testify/assert"
)

func TestHandleSNSConfirmation(t *testing.T) {
	called := 0
	snsSvc := newTestSNSService(func(r *request.Request) {
		if called > 0 {
			r.Error = errors.New("no no no")
		}
		called++
	})

	msg := &snsMessage{Token: "fafafaf", TopicARN: "faf/af/af"}
	status, err := handleSNSSubscriptionConfirmation(snsSvc, msg)
	assert.Equal(t, http.StatusOK, status)
	assert.Nil(t, err)

	msg = &snsMessage{Token: "fafafaf2", TopicARN: "faf/af/af2"}
	status, err = handleSNSSubscriptionConfirmation(snsSvc, msg)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.NotNil(t, err)
}

func TestHandleSNSNotification_EmptyMessage(t *testing.T) {
	status, err := handleSNSNotification(newTestRepo(), shushLog, newTestTokenGenerator(), &snsMessage{})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Regexp(t, "invalid json.+", err.Error())
}

func TestHandleSNSNotification_TestNotification(t *testing.T) {
	msg := &snsMessage{
		Message: `{"Event": "autoscaling:TEST_NOTIFICATION"}`,
	}
	status, err := handleSNSNotification(newTestRepo(), shushLog, newTestTokenGenerator(), msg)
	assert.Equal(t, http.StatusAccepted, status)
	assert.Nil(t, err)
}

func TestHandleSNSNotification_InstanceLaunching_InvalidPayload(t *testing.T) {
	msg := &snsMessage{
		Message: `{"LifecycleTransition": "autoscaling:EC2_INSTANCE_LAUNCHING"}`,
	}
	status, err := handleSNSNotification(newTestRepo(), shushLog, newTestTokenGenerator(), msg)
	assert.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Regexp(t, "missing required fields in lifecycle action.+", err.Error())
}

func TestHandleSNSNotification_InstanceLaunching(t *testing.T) {
	msg := &snsMessage{
		Message: strings.Join(strings.Split(`{
			"LifecycleTransition": "autoscaling:EC2_INSTANCE_LAUNCHING",
			"EC2InstanceId": "i-fafafaf",
			"LifecycleActionToken": "TOKEYTOKETOK",
			"AutoScalingGroupName": "cat-theatre-napkin-hose",
			"LifecycleHookName": "huzzah-9001"
		}`, ""), ""),
	}
	status, err := handleSNSNotification(newTestRepo(), shushLog, newTestTokenGenerator(), msg)
	assert.Equal(t, http.StatusOK, status)
	assert.Nil(t, err)
}

func TestHandleSNSNotification_InstanceTerminating(t *testing.T) {
	msg := &snsMessage{
		Message: strings.Join(strings.Split(`{
			"LifecycleTransition": "autoscaling:EC2_INSTANCE_TERMINATING",
			"EC2InstanceId": "i-fafafaf",
			"LifecycleActionToken": "TOKEYTOKETOK",
			"AutoScalingGroupName": "cat-theatre-napkin-hose",
			"LifecycleHookName": "huzzah-9001"
		}`, ""), ""),
	}
	status, err := handleSNSNotification(newTestRepo(), shushLog, newTestTokenGenerator(), msg)
	assert.Equal(t, http.StatusOK, status)
	assert.Nil(t, err)
}
