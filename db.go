package cyclist

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/sirupsen/logrus"
)

var (
	errEmptyInstanceID = errors.New("empty instance id")
	errEmptyEvent      = errors.New("empty event")
	errEmptyToken      = errors.New("empty token")
)

type redisConnGetter interface {
	Get() redis.Conn
}

type repo interface {
	setInstanceState(instanceID, state string) error
	fetchInstanceState(instanceID string) (string, error)
	wipeInstanceState(instanceID string) error

	storeInstanceEvent(instanceID, event string) error
	fetchInstanceEvent(instanceID, event string) (*lifecycleEvent, error)
	fetchInstanceEvents(instanceID string) ([]*lifecycleEvent, error)
	fetchAllInstanceEvents() (map[string][]*lifecycleEvent, error)

	storeInstanceLifecycleAction(la *lifecycleAction) error
	fetchInstanceLifecycleAction(transition, instanceID string) (*lifecycleAction, error)
	completeInstanceLifecycleAction(transition, instanceID string) error

	storeInstanceToken(instanceID, token string) error
	storeTempInstanceToken(instanceID, token string) error
	fetchInstanceToken(instanceID string) (string, error)
	fetchTempInstanceToken(instanceID string) (string, error)
}

type redisRepo struct {
	cg  redisConnGetter
	log logrus.FieldLogger

	instEventTTL           uint
	instLifecycleActionTTL uint
	instTempTokTTL         uint
	instTokTTL             uint
}

func (rr *redisRepo) setInstanceState(instanceID, state string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	instanceStateKey := fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID)
	conn := rr.cg.Get()
	defer rr.closeConn(conn)
	_, err := conn.Do("SET", instanceStateKey, state)
	return err
}

func (rr *redisRepo) fetchInstanceState(instanceID string) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", errEmptyInstanceID
	}

	conn := rr.cg.Get()
	defer rr.closeConn(conn)
	return redis.String(conn.Do("GET",
		fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID)))
}

func (rr *redisRepo) wipeInstanceState(instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	conn := rr.cg.Get()
	defer rr.closeConn(conn)
	_, err := conn.Do("DEL",
		fmt.Sprintf("%s:instance:%s:state", RedisNamespace, instanceID))
	return err
}

func (rr *redisRepo) storeInstanceEvent(instanceID, event string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	if strings.TrimSpace(event) == "" {
		return errEmptyEvent
	}

	return rr.hsetex(fmt.Sprintf("%s:instance:%s:events", RedisNamespace, instanceID),
		event, time.Now().UTC().Format(time.RFC3339Nano), rr.instEventTTL)
}

func (rr *redisRepo) fetchInstanceEvent(instanceID, event string) (*lifecycleEvent, error) {
	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	ts, err := redis.String(conn.Do("HGET", fmt.Sprintf("%s:instance:%s:events", RedisNamespace, instanceID), event))
	if err != nil {
		return nil, err
	}

	if ts == "" {
		return nil, fmt.Errorf("no %s event for instance %s", event, instanceID)
	}

	return newLifecycleEvent(event, ts), nil
}

func (rr *redisRepo) fetchInstanceEvents(instanceID string) ([]*lifecycleEvent, error) {
	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	return rr.fetchInstanceEventsWithConn(conn, instanceID)
}

func (rr *redisRepo) fetchInstanceEventsWithConn(conn redis.Conn, instanceID string) ([]*lifecycleEvent, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, errEmptyInstanceID
	}

	raw, err := redis.StringMap(conn.Do("HGETALL", fmt.Sprintf("%s:instance:%s:events", RedisNamespace, instanceID)))
	if err != nil {
		return nil, err
	}

	events := []*lifecycleEvent{}

	revMap := map[string]string{}
	revMapKeys := []string{}

	for event, ts := range raw {
		key := fmt.Sprintf("%s::%s", ts, event)
		revMap[key] = event
		revMapKeys = append(revMapKeys, key)
	}

	sort.Strings(revMapKeys)

	for _, key := range revMapKeys {
		keyParts := strings.SplitN(key, "::", 2)
		events = append(events, newLifecycleEvent(revMap[key], keyParts[0]))
	}

	return events, nil
}

func (rr *redisRepo) fetchAllInstanceEvents() (map[string][]*lifecycleEvent, error) {
	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	instanceEventKeys, err := rr.scanKeysPattern(fmt.Sprintf("%s:instance:*:events", RedisNamespace))
	if err != nil {
		return nil, err
	}

	res := map[string][]*lifecycleEvent{}
	for _, key := range instanceEventKeys {
		keyParts := strings.Split(key, ":")
		if len(keyParts) != 4 {
			return nil, fmt.Errorf("invalid events key %q", key)
		}

		instanceID := keyParts[2]
		events, err := rr.fetchInstanceEventsWithConn(conn, instanceID)
		if err != nil {
			return nil, err
		}

		res[instanceID] = events
	}

	return res, nil
}

func (rr *redisRepo) scanKeysPattern(pattern string) ([]string, error) {
	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	fullResults := []string{}
	keysSlice := []interface{}{}
	cursorSlice := []byte{}
	cursor := uint64(0)
	keyBytes := []byte{}
	ok := false

	for {
		raw, err := redis.Values(conn.Do("SCAN", cursor, "MATCH", pattern))
		if err != nil {
			return fullResults, err
		}

		if len(raw) < 2 {
			return fullResults, fmt.Errorf("unexpected scan result length=%d", len(raw))
		}

		cursorSlice, ok = raw[0].([]byte)
		if !ok {
			return fullResults, fmt.Errorf("scan cursor was not []byte but %T", raw[0])
		}

		if len(cursorSlice) == 0 {
			return fullResults, fmt.Errorf("scan cursor is empty")
		}

		cursor, err = strconv.ParseUint(string(cursorSlice), 10, 0)
		if err != nil {
			return fullResults, err
		}

		keysSlice, ok = raw[1].([]interface{})
		if !ok {
			return fullResults, fmt.Errorf("scan results not []interface{} but %T", raw[1])
		}

		for _, b := range keysSlice {
			keyBytes, ok = b.([]byte)
			if !ok {
				return fullResults, fmt.Errorf("scan results contain non-string key %T", b)
			}

			fullResults = append(fullResults, string(keyBytes))
		}

		if cursor == 0 {
			return fullResults, nil
		}
	}
}

func (rr *redisRepo) storeInstanceLifecycleAction(a *lifecycleAction) error {
	if a.LifecycleTransition == "" || a.EC2InstanceID == "" ||
		a.LifecycleActionToken == "" || a.AutoScalingGroupName == "" ||
		a.LifecycleHookName == "" {
		return fmt.Errorf("missing required fields in lifecycle action: %+v", a)
	}

	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	err := conn.Send("MULTI")
	if err != nil {
		return err
	}

	transition := a.Transition()
	hashKey := fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, a.EC2InstanceID)

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

	err = conn.Send("EXPIRE", hashKey, rr.instLifecycleActionTTL)
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
	defer rr.closeConn(conn)

	attrs, err := redis.Values(conn.Do("HGETALL", fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, instanceID)))
	if err != nil {
		return nil, err
	}

	if len(attrs) == 0 {
		return nil, nil
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

func (rr *redisRepo) completeInstanceLifecycleAction(transition, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	_, err := conn.Do("HSET",
		fmt.Sprintf("%s:instance_%s:%s", RedisNamespace, transition, instanceID),
		"completed", true)
	return err
}

func (rr *redisRepo) storeInstanceToken(instanceID, token string) error {
	return rr.storeInstanceTokenfTTL("%s:instance:%s:token", instanceID, token, rr.instTokTTL)
}

func (rr *redisRepo) storeTempInstanceToken(instanceID, token string) error {
	return rr.storeInstanceTokenfTTL("%s:instance:%s:tmptoken", instanceID, token, rr.instTempTokTTL)
}

func (rr *redisRepo) storeInstanceTokenfTTL(fmtString, instanceID, token string, ttl uint) error {
	if strings.TrimSpace(instanceID) == "" {
		return errEmptyInstanceID
	}

	token = strings.TrimSpace(token)

	if token == "" {
		return errEmptyToken
	}

	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	_, err := conn.Do("SETEX",
		fmt.Sprintf(fmtString, RedisNamespace, instanceID), ttl, token)
	return err
}

func (rr *redisRepo) fetchInstanceToken(instanceID string) (string, error) {
	return rr.fetchInstanceTokenfTTL("%s:instance:%s:token", instanceID, rr.instTokTTL)
}

func (rr *redisRepo) fetchTempInstanceToken(instanceID string) (string, error) {
	return rr.fetchInstanceTokenfTTL("%s:instance:%s:tmptoken", instanceID, uint(0))
}

func (rr *redisRepo) fetchInstanceTokenfTTL(fmtString, instanceID string, ttl uint) (string, error) {
	if strings.TrimSpace(instanceID) == "" {
		return "", errEmptyInstanceID
	}

	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	key := fmt.Sprintf(fmtString, RedisNamespace, instanceID)
	token, err := redis.String(conn.Do("GET", key))
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(token) == "" {
		return "", errEmptyToken
	}

	if ttl > uint(0) {
		_, err = conn.Do("EXPIRE", key, ttl)
		if err != nil {
			return "", err
		}
	}

	return token, nil
}

func (rr *redisRepo) closeConn(conn redis.Conn) {
	err := conn.Close()
	if err != nil && rr.log != nil {
		rr.log.WithField("err", err).Error("failed to close redis conn")
	}
}

func (rr *redisRepo) hsetex(hashKey, key, value string, ttl uint) error {
	conn := rr.cg.Get()
	defer rr.closeConn(conn)

	err := conn.Send("MULTI")
	if err != nil {
		return err
	}

	err = conn.Send("HSET", hashKey, key, value)
	if err != nil {
		conn.Do("DISCARD")
		return err
	}

	err = conn.Send("EXPIRE", hashKey, fmt.Sprintf("%d", ttl))
	if err != nil {
		conn.Do("DISCARD")
		return err
	}

	_, err = conn.Do("EXEC")
	return err
}

func buildRedisPool(redisURL string) redisConnGetter {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: time.Minute,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(redisURL)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}
