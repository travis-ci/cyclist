package cyclist

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/autoscaling/autoscalingiface"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/garyburd/redigo/redis"
	"github.com/rafaeljusto/redigomock"
)

var (
	shushLog = func() *logrus.Logger {
		l := logrus.New()
		l.Level = logrus.FatalLevel
		return l
	}()
)

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
	la map[string]*lifecycleAction
}

func newTestRepo() *testRepo {
	return &testRepo{
		s:  map[string]string{},
		la: map[string]*lifecycleAction{},
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

func (tr *testRepo) wipeInstanceLifecycleAction(transition, instanceID string) error {
	key := fmt.Sprintf("%s:%s", transition, instanceID)
	if _, ok := tr.la[key]; ok {
		delete(tr.la, key)
		return nil
	}
	return fmt.Errorf("no lifecycle action found for transition '%s', instance ID '%s'", transition, instanceID)
}

func newTestSNSService(f func(*request.Request)) snsiface.SNSAPI {
	svc := sns.New(session.New(), nil)
	svc.Handlers.Clear()
	if f == nil {
		f = func(r *request.Request) {
			shushLog.WithField("request", r).Info("got this for ya")
		}
	}
	svc.Handlers.Build.PushBack(f)
	return svc
}

func newTestAutosScalingService(f func(*request.Request)) autoscalingiface.AutoScalingAPI {
	svc := autoscaling.New(session.New(), nil)
	svc.Handlers.Clear()
	if f == nil {
		f = func(r *request.Request) {
			shushLog.WithField("request", r).Info("got this for ya")
		}
	}
	svc.Handlers.Build.PushBack(f)
	return svc
}
