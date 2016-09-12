package cyclist

import "encoding/json"

type snsMessage struct {
	Message   string
	MessageID string `json:"MessageId"`
	Token     string
	TopicARN  string `json:"TopicArn"`
	Type      string
}

func (m *snsMessage) lifecycleAction() (*lifecycleAction, error) {
	a := &lifecycleAction{}
	err := json.Unmarshal([]byte(m.Message), a)
	if err != nil {
		return nil, err
	}

	return a, nil
}
