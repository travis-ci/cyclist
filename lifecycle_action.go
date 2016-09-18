package cyclist

import "strings"

type lifecycleAction struct {
	Event                string
	AutoScalingGroupName string `redis:"auto_scaling_group_name"`
	Service              string
	Time                 string
	AccountID            string `json:"AccountId"`
	LifecycleTransition  string
	RequestID            string `json:"RequestId"`
	LifecycleActionToken string `redis:"lifecycle_action_token"`
	EC2InstanceID        string `json:"EC2InstanceId"`
	LifecycleHookName    string `redis:"lifecycle_hook_name"`
}

func (la *lifecycleAction) Transition() string {
	return strings.ToLower(strings.Replace(la.LifecycleTransition, "autoscaling:EC2_INSTANCE_", "", -1))
}
