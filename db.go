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
	dbPool redisConnGetter

	errEmptyInstanceID = errors.New("empty instance id")
)

type redisConnGetter interface {
	Get() redis.Conn
}

func buildRedisPool(redisURL string) (*redis.Pool, error) {
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

func setInstanceState(conn redis.Conn, instanceID, state string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	instanceStateKey := fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID)
	_, err := conn.Do("SET", instanceStateKey, state)
	return err
}

func fetchInstanceState(conn redis.Conn, instanceID string) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", errEmptyInstanceID
	}

	return redis.String(conn.Do("GET",
		fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID)))
}

func storeInstanceLifecycleAction(conn redis.Conn, a *lifecycleAction) error {
	if a.LifecycleTransition == "" || a.EC2InstanceID == "" ||
		a.LifecycleActionToken == "" || a.AutoScalingGroupName == "" ||
		a.LifecycleHookName == "" {
		return fmt.Errorf("missing required fields in lifecycle action: %+v", a)
	}
	err := conn.Send("MULTI")
	if err != nil {
		return err
	}

	transition := strings.ToLower(strings.Replace(a.LifecycleTransition, "autoscaling:EC2_INSTANCE_", "", 1))
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

func fetchInstanceLifecycleAction(conn redis.Conn, transition, instanceID string) (*lifecycleAction, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, errEmptyInstanceID
	}

	exists, err := redis.Bool(conn.Do("SISMEMBER", fmt.Sprintf("%s:instance_%s", RedisNamespace, transition), instanceID))
	if !exists {
		return nil, nil
	}

	attrs, err := redis.Values(conn.Do("HGETALL", fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, instanceID)))
	if err != nil {
		return nil, err
	}

	ala := &lifecycleAction{}
	err = redis.ScanStruct(attrs, ala)
	return ala, err
}

func wipeInstanceLifecycleAction(conn redis.Conn, transition, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

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
