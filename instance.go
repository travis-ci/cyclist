package cyclist

// Instance is the internal representation of an EC2 instance
type Instance struct {
	InstanceID    string `json:"id" redis:"instance_id"`
	ExpectedState string `json:"expected_state,omitempty" redis:"expected_state"`
}
