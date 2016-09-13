package cyclist

import (
	"errors"
	"testing"

	"github.com/rafaeljusto/redigomock"
	"github.com/stretchr/testify/assert"
)

func TestRedisRepo(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}
	conn := rr.cg.Get()
	assert.NotNil(t, conn)
}

func TestRedisRepo_setInstanceState(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("SET", "cyclist:instance:i-fafafaf:state", "denial").Expect("OK!")

	err := rr.setInstanceState("i-fafafaf", "denial")
	assert.Nil(t, err)
}

func TestRedisRepo_setInstanceState_WithEmptyInstanceID(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	err := rr.setInstanceState("", "denial")
	assert.NotNil(t, err)
}

func TestRedisRepo_fetchInstanceState(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("GET", "cyclist:instance:i-fafafaf:state").Expect("catatonia")

	state, err := rr.fetchInstanceState("i-fafafaf")
	assert.Nil(t, err)
	assert.Equal(t, "catatonia", state)
}

func TestRedisRepo_fetchInstanceState_WithEmptyInstanceID(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	state, err := rr.fetchInstanceState("")
	assert.NotNil(t, err)
	assert.Equal(t, "", state)
}

func TestRedisRepo_wipeInstanceState(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("DEL", "cyclist:instance:i-fafafaf:state").Expect("OK!")

	err := rr.wipeInstanceState("i-fafafaf")
	assert.Nil(t, err)
}

func TestRedisRepo_wipeInstanceState_WithEmptyInstanceID(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	err := rr.wipeInstanceState("")
	assert.NotNil(t, err)
}

func TestRedisRepo_storeInstanceLifecycleAction(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SADD", "cyclist:instance_loathing", "i-fafafaf").Expect("OK!")
	conn.Command("HMSET", "cyclist:instance_loathing:i-fafafaf",
		"lifecycle_action_token", "TOKEYTOKETOK",
		"auto_scaling_group_name", "menial-jar-legs",
		"lifecycle_hook_name", "frazzled-top-zipper").Expect("OK!")
	conn.Command("EXEC").Expect("OK!")

	err := rr.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LOATHING",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "menial-jar-legs",
		LifecycleHookName:    "frazzled-top-zipper",
	})

	assert.Nil(t, err)
}

func TestRedisRepo_storeInstanceLifecycleAction_WithInvalidLifecycleAction(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}
	err := rr.storeInstanceLifecycleAction(&lifecycleAction{})
	assert.NotNil(t, err)
}

func TestRedisRepo_storeInstanceLifecycleAction_WithFailedMulti(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").ExpectError(errors.New("not do"))

	err := rr.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LOATHING",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "menial-jar-legs",
		LifecycleHookName:    "frazzled-top-zipper",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "not do", err.Error())
}

func TestRedisRepo_storeInstanceLifecycleAction_WithFailedSadd(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SADD", "cyclist:instance_loathing", "i-fafafaf").ExpectError(errors.New("no sads"))
	conn.Command("DISCARD").Expect("OK!")

	err := rr.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LOATHING",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "menial-jar-legs",
		LifecycleHookName:    "frazzled-top-zipper",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "no sads", err.Error())
}

func TestRedisRepo_storeInstanceLifecycleAction_WithFailedHmset(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SADD", "cyclist:instance_loathing", "i-fafafaf").Expect("OK!")
	conn.Command("HMSET", "cyclist:instance_loathing:i-fafafaf",
		"lifecycle_action_token", "TOKEYTOKETOK",
		"auto_scaling_group_name", "menial-jar-legs",
		"lifecycle_hook_name", "frazzled-top-zipper").ExpectError(errors.New("no hmm sets"))
	conn.Command("DISCARD").Expect("OK!")

	err := rr.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LOATHING",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "menial-jar-legs",
		LifecycleHookName:    "frazzled-top-zipper",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "no hmm sets", err.Error())
}

func TestRedisRepo_storeInstanceLifecycleAction_WithFailedExec(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SADD", "cyclist:instance_loathing", "i-fafafaf").Expect("OK!")
	conn.Command("HMSET", "cyclist:instance_loathing:i-fafafaf",
		"lifecycle_action_token", "TOKEYTOKETOK",
		"auto_scaling_group_name", "menial-jar-legs",
		"lifecycle_hook_name", "frazzled-top-zipper").Expect("OK!")
	conn.Command("EXEC").ExpectError(errors.New("not exectly"))

	err := rr.storeInstanceLifecycleAction(&lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LOATHING",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "menial-jar-legs",
		LifecycleHookName:    "frazzled-top-zipper",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "not exectly", err.Error())
}

func TestRedisRepo_fetchInstanceLifecycleAction(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("SISMEMBER", "cyclist:instance_larping", "i-fafafaf").Expect(int64(1))
	conn.Command("HGETALL", "cyclist:instance_larping:i-fafafaf").ExpectMap(map[string]string{
		"lifecycle_action_token":  "TOKEYTOKETOK",
		"auto_scaling_group_name": "menial-jar-legs",
		"lifecycle_hook_name":     "frazzled-top-zipper",
	})

	la, err := rr.fetchInstanceLifecycleAction("larping", "i-fafafaf")
	assert.Nil(t, err)
	assert.Equal(t, &lifecycleAction{
		LifecycleTransition:  "autoscaling:EC2_INSTANCE_LARPING",
		EC2InstanceID:        "i-fafafaf",
		LifecycleActionToken: "TOKEYTOKETOK",
		AutoScalingGroupName: "menial-jar-legs",
		LifecycleHookName:    "frazzled-top-zipper",
	}, la)
}

func TestRedisRepo_fetchInstanceLifecycleAction_WithEmptyInstanceID(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	la, err := rr.fetchInstanceLifecycleAction("looming", "")
	assert.NotNil(t, err)
	assert.Nil(t, la)
}

func TestRedisRepo_fetchInstanceLifecycleAction_WithFailedSismember(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("SISMEMBER", "cyclist:instance_larping", "i-fafafaf").Expect(int64(0))

	la, err := rr.fetchInstanceLifecycleAction("larping", "i-fafafaf")
	assert.Nil(t, la)
	assert.NotNil(t, err)
	assert.Equal(t,
		"instance 'i-fafafaf' not in set for transition 'larping'", err.Error())
}

func TestRedisRepo_fetchInstanceLifecycleAction_WithFailedHgetall(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("SISMEMBER", "cyclist:instance_larping", "i-fafafaf").Expect(int64(1))
	conn.Command("HGETALL", "cyclist:instance_larping:i-fafafaf").ExpectError(errors.New("not so getall"))

	la, err := rr.fetchInstanceLifecycleAction("larping", "i-fafafaf")
	assert.Nil(t, la)
	assert.NotNil(t, err)
	assert.Equal(t, "not so getall", err.Error())
}

func TestRedisRepo_wipeInstanceLifecycleAction(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SREM", "cyclist:instance_fuming", "i-fafafaf").Expect("OK!")
	conn.Command("DEL", "cyclist:instance_fuming:i-fafafaf").Expect("OK!")
	conn.Command("EXEC").Expect("OK!")

	err := rr.wipeInstanceLifecycleAction("fuming", "i-fafafaf")
	assert.Nil(t, err)
}

func TestRedisRepo_wipeInstanceLifecycleAction_WithEmptyInstanceID(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	err := rr.wipeInstanceLifecycleAction("fuming", "")
	assert.NotNil(t, err)
	assert.Equal(t, errEmptyInstanceID, err)
}

func TestRedisRepo_wipeInstanceLifecycleAction_WithFailedMulti(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").ExpectError(errors.New("multi-nope"))

	err := rr.wipeInstanceLifecycleAction("fuming", "i-fafafaf")
	assert.NotNil(t, err)
	assert.Equal(t, "multi-nope", err.Error())
}

func TestRedisRepo_wipeInstanceLifecycleAction_WithFailedSrem(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SREM", "cyclist:instance_fuming", "i-fafafaf").ExpectError(errors.New("not srem"))
	conn.Command("DISCARD").Expect("OK!")

	err := rr.wipeInstanceLifecycleAction("fuming", "i-fafafaf")
	assert.NotNil(t, err)
	assert.Equal(t, "not srem", err.Error())
}

func TestRedisRepo_wipeInstanceLifecycleAction_WithFailedDel(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SREM", "cyclist:instance_fuming", "i-fafafaf").Expect("OK!")
	conn.Command("DEL", "cyclist:instance_fuming:i-fafafaf").ExpectError(errors.New("control alt"))
	conn.Command("DISCARD").Expect("OK!")

	err := rr.wipeInstanceLifecycleAction("fuming", "i-fafafaf")
	assert.NotNil(t, err)
	assert.Equal(t, "control alt", err.Error())
}

func TestRedisRepo_wipeInstanceLifecycleAction_WithFailedExec(t *testing.T) {
	rr := &redisRepo{cg: &testRedisConnGetter{}}

	conn := rr.cg.Get().(*redigomock.Conn)
	conn.Command("MULTI").Expect("OK!")
	conn.Command("SREM", "cyclist:instance_fuming", "i-fafafaf").Expect("OK!")
	conn.Command("DEL", "cyclist:instance_fuming:i-fafafaf").Expect("OK!")
	conn.Command("EXEC").ExpectError(errors.New("def not exectly"))

	err := rr.wipeInstanceLifecycleAction("fuming", "i-fafafaf")
	assert.NotNil(t, err)
	assert.Equal(t, "def not exectly", err.Error())
}
