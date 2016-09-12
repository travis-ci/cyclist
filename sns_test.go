package cyclist

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleSNSConfirmation(t *testing.T) {
	oldSg := sg
	defer func() { sg = oldSg }()

	sg = &testSNSGetter{ErrorConfirmSubscription: false}
	msg := &snsMessage{Token: "fafafaf", TopicARN: "faf/af/af"}
	status, err := handleSNSConfirmation(msg, "nz-mordor-1")
	assert.Equal(t, http.StatusOK, status)
	assert.Nil(t, err)

	sg = &testSNSGetter{ErrorConfirmSubscription: true}
	msg = &snsMessage{Token: "fafafaf2", TopicARN: "faf/af/af2"}
	status, err = handleSNSConfirmation(msg, "nz-mordor-1")
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.NotNil(t, err)
}

func TestHandleSNSNotification_EmptyMessage(t *testing.T) {
	oldDbPool := dbPool
	defer func() { dbPool = oldDbPool }()

	dbPool = &testRedisConnGetter{}
	status, err := handleSNSNotification(&snsMessage{}, "nz-mordor-1")
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Regexp(t, "invalid json.+", err.Error())
}

func TestHandleSNSNotification_TestNotification(t *testing.T) {
	oldDbPool := dbPool
	defer func() { dbPool = oldDbPool }()

	dbPool = &testRedisConnGetter{}
	msg := &snsMessage{
		Message: `{"Event": "autoscaling:TEST_NOTIFICATION"}`,
	}
	status, err := handleSNSNotification(msg, "nz-mordor-1")
	assert.Equal(t, http.StatusAccepted, status)
	assert.Nil(t, err)
}

func TestHandleSNSNotification_InstanceLaunching_InvalidPayload(t *testing.T) {
	oldDbPool := dbPool
	defer func() { dbPool = oldDbPool }()

	dbPool = &testRedisConnGetter{}
	msg := &snsMessage{
		Message: `{"LifecycleTransition": "autoscaling:EC2_INSTANCE_LAUNCHING"}`,
	}
	status, err := handleSNSNotification(msg, "nz-mordor-1")
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Regexp(t, "missing required fields in lifecycle action.+", err.Error())
}

func TestHandleSNSNotification_InstanceLaunching(t *testing.T) {
	oldDbPool := dbPool
	defer func() { dbPool = oldDbPool }()

	trgc := &testRedisConnGetter{}
	_ = trgc.Get()
	conn := trgc.Conn
	dbPool = trgc

	msg := &snsMessage{
		Message: strings.Join(strings.Split(`{
			"LifecycleTransition": "autoscaling:EC2_INSTANCE_LAUNCHING",
			"EC2InstanceId": "i-fafafaf",
			"LifecycleActionToken": "TOKEYTOKETOK",
			"AutoScalingGroupName": "cat-theatre-napkin-hose",
			"LifecycleHookName": "huzzah-9001"
		}`, ""), ""),
	}
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SADD", "cyclist:instance_launching", "i-fafafaf").Expect("OK!")
	conn.Command("HMSET",
		"cyclist:instance_launching:i-fafafaf",
		"lifecycle_action_token", "TOKEYTOKETOK",
		"auto_scaling_group_name", "cat-theatre-napkin-hose",
		"lifecycle_hook_name", "huzzah-9001",
	).Expect("OK!")
	conn.Command("EXEC").Expect("OK!")

	status, err := handleSNSNotification(msg, "nz-mordor-1")
	assert.Equal(t, http.StatusOK, status)
	assert.Nil(t, err)
}

func TestHandleSNSNotification_InstanceTerminating(t *testing.T) {
	oldDbPool := dbPool
	defer func() { dbPool = oldDbPool }()

	trgc := &testRedisConnGetter{}
	_ = trgc.Get()
	conn := trgc.Conn
	dbPool = trgc

	msg := &snsMessage{
		Message: strings.Join(strings.Split(`{
			"LifecycleTransition": "autoscaling:EC2_INSTANCE_TERMINATING",
			"EC2InstanceId": "i-fafafaf",
			"LifecycleActionToken": "TOKEYTOKETOK",
			"AutoScalingGroupName": "cat-theatre-napkin-hose",
			"LifecycleHookName": "huzzah-9001"
		}`, ""), ""),
	}
	conn.Command("HMSET", "cyclist:instance:i-fafafaf", "expected_state", "down")
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SADD", "cyclist:instance_terminating", "i-fafafaf").Expect("OK!")
	conn.Command("HMSET",
		"cyclist:instance_terminating:i-fafafaf",
		"lifecycle_action_token", "TOKEYTOKETOK",
		"auto_scaling_group_name", "cat-theatre-napkin-hose",
		"lifecycle_hook_name", "huzzah-9001",
	).Expect("OK!")
	conn.Command("EXEC").Expect("OK!")

	status, err := handleSNSNotification(msg, "nz-mordor-1")
	assert.Equal(t, http.StatusOK, status)
	assert.Nil(t, err)
}
