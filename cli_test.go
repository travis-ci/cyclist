package cyclist

import (
	"bytes"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestBuildLog(t *testing.T) {
	buf := &bytes.Buffer{}
	defaultLogOut = buf
	defer func() { defaultLogOut = os.Stdout }()

	log := buildLog(false)
	assert.NotNil(t, log)
	assert.Equal(t, logrus.InfoLevel, log.(*logrus.Logger).Level)

	log = buildLog(true)
	assert.NotNil(t, log)
	assert.Equal(t, logrus.DebugLevel, log.(*logrus.Logger).Level)
}
