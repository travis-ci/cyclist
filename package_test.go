package cyclist

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/garyburd/redigo/redis"
	"github.com/rafaeljusto/redigomock"
	"github.com/stretchr/testify/assert"
	"gopkg.in/urfave/cli.v2"
)

var (
	shushLog = func() logrus.FieldLogger {
		l := logrus.New()
		l.Level = logrus.FatalLevel
		return l
	}()
)

func TestCustomVersionPrinter(t *testing.T) {
	assert.Equal(t, fmt.Sprintf("%p", cli.VersionPrinter), fmt.Sprintf("%p", customVersionPrinter))
	buf := &bytes.Buffer{}
	ctx := cli.NewContext(nil, nil, nil)
	ctx.App = &cli.App{Name: "hay", Writer: buf}
	customVersionPrinter(ctx)
	assert.Regexp(t, "hay v=.* rev=.* d=.*", buf.String())
}

type testRedisConnGetter struct {
	Conn *redigomock.Conn
}

func (trcg *testRedisConnGetter) Get() redis.Conn {
	if trcg.Conn == nil {
		trcg.Conn = redigomock.NewConn()
	}
	return trcg.Conn
}

type testRepo struct {
	s  map[string]string
	e  map[string]map[string]*lifecycleEvent
	la map[string]*lifecycleAction
	t  map[string]string
	tt map[string]string
}

func newTestRepo() *testRepo {
	return &testRepo{
		s:  map[string]string{},
		e:  map[string]map[string]*lifecycleEvent{},
		la: map[string]*lifecycleAction{},
		t:  map[string]string{},
		tt: map[string]string{},
	}
}

func (tr *testRepo) setInstanceState(instanceID, state string) error {
	tr.s[instanceID] = state
	return nil
}

func (tr *testRepo) fetchInstanceState(instanceID string) (string, error) {
	if state, ok := tr.s[instanceID]; ok {
		return state, nil
	}

	return "", fmt.Errorf("no state for instance '%s'", instanceID)
}

func (tr *testRepo) wipeInstanceState(instanceID string) error {
	if _, ok := tr.s[instanceID]; ok {
		delete(tr.s, instanceID)
		return nil
	}

	return fmt.Errorf("no state for instance '%s'", instanceID)
}

func (tr *testRepo) storeInstanceEvent(instanceID, event string) error {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	if _, ok := tr.e[instanceID]; !ok {
		tr.e[instanceID] = map[string]*lifecycleEvent{}
	}
	tr.e[instanceID][event] = newLifecycleEvent(event, ts)
	return nil
}

func (tr *testRepo) fetchInstanceEvent(instanceID, event string) (*lifecycleEvent, error) {
	eventsMap, ok := tr.e[instanceID]
	if !ok {
		return nil, fmt.Errorf("no events for instance '%s'", instanceID)
	}

	le, ok := eventsMap[event]
	if !ok {
		return nil, fmt.Errorf("no '%s' event for instance '%s'", event, instanceID)
	}

	return le, nil
}

func (tr *testRepo) fetchInstanceEvents(instanceID string) ([]*lifecycleEvent, error) {
	eventsMap, ok := tr.e[instanceID]
	if !ok {
		return nil, fmt.Errorf("no events for instance '%s'", instanceID)
	}

	events := []*lifecycleEvent{}

	for _, event := range eventsMap {
		events = append(events, event)
	}

	return events, nil
}

func (tr *testRepo) fetchAllInstanceEvents() (map[string][]*lifecycleEvent, error) {
	ret := map[string][]*lifecycleEvent{}

	for instanceID, eventsMap := range tr.e {
		ret[instanceID] = []*lifecycleEvent{}
		for _, event := range eventsMap {
			ret[instanceID] = append(ret[instanceID], event)
		}
	}

	return ret, nil
}

func (tr *testRepo) storeInstanceLifecycleAction(la *lifecycleAction) error {
	if la.LifecycleTransition == "" || la.EC2InstanceID == "" ||
		la.LifecycleActionToken == "" || la.AutoScalingGroupName == "" ||
		la.LifecycleHookName == "" {
		return fmt.Errorf("missing required fields in lifecycle action: %+v", la)
	}

	tr.la[fmt.Sprintf("%s:%s", la.Transition(), la.EC2InstanceID)] = la
	return nil
}

func (tr *testRepo) fetchInstanceLifecycleAction(transition, instanceID string) (*lifecycleAction, error) {
	if la, ok := tr.la[fmt.Sprintf("%s:%s", transition, instanceID)]; ok {
		return la, nil
	}
	return nil, fmt.Errorf("no lifecycle action found for transition '%s', instance ID '%s'", transition, instanceID)
}

func (tr *testRepo) completeInstanceLifecycleAction(transition, instanceID string) error {
	key := fmt.Sprintf("%s:%s", transition, instanceID)
	if _, ok := tr.la[key]; ok {
		tr.la[key].Completed = true
		return nil
	}
	return fmt.Errorf("no lifecycle action found for transition '%s', instance ID '%s'", transition, instanceID)
}

func (tr *testRepo) fetchInstanceToken(instanceID string) (string, error) {
	if tok, ok := tr.t[instanceID]; ok {
		return tok, nil
	}

	return "", fmt.Errorf("no token for instance '%s'", instanceID)
}

func (tr *testRepo) storeInstanceToken(instanceID, token string) error {
	tr.t[instanceID] = token
	return nil
}

func (tr *testRepo) fetchTempInstanceToken(instanceID string) (string, error) {
	if tok, ok := tr.tt[instanceID]; ok {
		return tok, nil
	}

	return "", fmt.Errorf("no token for instance '%s'", instanceID)
}

func (tr *testRepo) storeTempInstanceToken(instanceID, token string) error {
	tr.tt[instanceID] = token
	return nil
}

func newTestSNSService(f func(*request.Request)) snsiface.SNSAPI {
	svc := sns.New(session.New(), aws.NewConfig().WithRegion("nz-isengard-1"))
	svc.Handlers.Clear()
	if f == nil {
		f = func(r *request.Request) {
			shushLog.WithField("request", r).Info("got this for ya")
		}
	}
	svc.Handlers.Build.PushBack(f)
	return svc
}

func newTestAutoScalingService(f func(*request.Request)) autoscalingiface.AutoScalingAPI {
	svc := autoscaling.New(session.New(), aws.NewConfig().WithRegion("nz-isengard-1"))
	svc.Handlers.Clear()
	if f == nil {
		f = func(r *request.Request) {
			shushLog.WithField("request", r).Info("got this for ya")
		}
	}
	svc.Handlers.Build.PushBack(f)
	return svc
}

type testTokenGenerator struct{}

func (ttg *testTokenGenerator) GenerateToken() string {
	return "ffffffff-aaaa-ffff-aaaa-ffffffffffff"
}

func newTestTokenGenerator() tokenGenerator {
	return &testTokenGenerator{}
}
