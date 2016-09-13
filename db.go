package cyclist

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

var (
	errEmptyInstanceID = errors.New("empty instance id")
)

type redisConnGetter interface {
	Get() redis.Conn
}

type repo interface {
	setInstanceState(instanceID, state string) error
	fetchInstanceState(instanceID string) (string, error)
	wipeInstanceState(instanceID string) error
	storeInstanceLifecycleAction(la *lifecycleAction) error
	fetchInstanceLifecycleAction(transition, instanceID string) (*lifecycleAction, error)
	wipeInstanceLifecycleAction(transition, instanceID string) error
}

type redisRepo struct {
	cg redisConnGetter
}

func (rr *redisRepo) setInstanceState(instanceID, state string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	instanceStateKey := fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID)
	_, err := rr.cg.Get().Do("SET", instanceStateKey, state)
	return err
}

func (rr *redisRepo) fetchInstanceState(instanceID string) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", errEmptyInstanceID
	}

	return redis.String(rr.cg.Get().Do("GET",
		fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID)))
}

func (rr *redisRepo) wipeInstanceState(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	_, err := rr.cg.Get().Do("DEL",
		fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID))
	return err
}

func (rr *redisRepo) storeInstanceLifecycleAction(a *lifecycleAction) error {
	if a.LifecycleTransition == "" || a.EC2InstanceID == "" ||
		a.LifecycleActionToken == "" || a.AutoScalingGroupName == "" ||
		a.LifecycleHookName == "" {
		return fmt.Errorf("missing required fields in lifecycle action: %+v", a)
	}

	conn := rr.cg.Get()
	defer func() { _ = conn.Close() }()

	err := conn.Send("MULTI")
	if err != nil {
		return err
	}

	transition := a.Transition()
	instSetKey := fmt.Sprintf("%s:instance_%s", RedisNamespace, transition)
	hashKey := fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, a.EC2InstanceID)

	err = conn.Send("SADD", instSetKey, a.EC2InstanceID)
	if err != nil {
		conn.Do("DISCARD")
		return err
	}

	hmSet := []interface{}{
		hashKey,
		"lifecycle_action_token", a.LifecycleActionToken,
		"auto_scaling_group_name", a.AutoScalingGroupName,
		"lifecycle_hook_name", a.LifecycleHookName,
	}

	err = conn.Send("HMSET", hmSet...)
	if err != nil {
		conn.Do("DISCARD")
		return err
	}

	_, err = conn.Do("EXEC")
	return err
}

func (rr *redisRepo) fetchInstanceLifecycleAction(transition, instanceID string) (*lifecycleAction, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, errEmptyInstanceID
	}

	conn := rr.cg.Get()
	defer func() { _ = conn.Close() }()

	exists, err := redis.Bool(conn.Do("SISMEMBER", fmt.Sprintf("%s:instance_%s", RedisNamespace, transition), instanceID))
	if !exists {
		return nil, fmt.Errorf("instance '%s' not in set for transition '%s'", instanceID, transition)
	}

	if err != nil {
		return nil, err
	}

	attrs, err := redis.Values(conn.Do("HGETALL", fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, instanceID)))
	if err != nil {
		return nil, err
	}

	ala := &lifecycleAction{}
	err = redis.ScanStruct(attrs, ala)
	if err != nil {
		return nil, err
	}

	ala.LifecycleTransition = fmt.Sprintf("autoscaling:EC2_INSTANCE_%s", strings.ToUpper(transition))
	ala.EC2InstanceID = instanceID
	return ala, nil
}

func (rr *redisRepo) wipeInstanceLifecycleAction(transition, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	conn := rr.cg.Get()
	defer func() { _ = conn.Close() }()

	err := conn.Send("MULTI")
	if err != nil {
		return err
	}

	err = conn.Send("SREM", fmt.Sprintf("%s:instance_%s", RedisNamespace, transition), instanceID)
	if err != nil {
		conn.Do("DISCARD")
		return err
	}

	err = conn.Send("DEL", fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, instanceID))
	if err != nil {
		conn.Do("DISCARD")
		return err
	}

	_, err = conn.Do("EXEC")
	return err
}

func buildRedisPool(redisURL string) (redisConnGetter, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return nil, err
	}

	pool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", u.Host)
			if err != nil {
				return nil, err
			}
			if u.User == nil {
				return c, err
			}
			if auth, ok := u.User.Password(); ok {
				if _, err := c.Do("AUTH", auth); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	return pool, nil
}
