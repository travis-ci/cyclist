package cyclist

type lifecycleTransition struct {
	ID         string `json:"id,omitempty"`
	InstanceID string `json:"instance_id"`
	Transition string `json:"transition"`
}
