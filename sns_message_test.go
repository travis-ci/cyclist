package cyclist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnsMessage_lifecycleAction(t *testing.T) {
	m := &snsMessage{
		Message: `{}`,
	}

	la, err := m.lifecycleAction()
	assert.Nil(t, err)
	assert.NotNil(t, la)

	m = &snsMessage{
		Message: `{bogus`,
	}

	la, err = m.lifecycleAction()
	assert.NotNil(t, err)
	assert.Nil(t, la)
}
