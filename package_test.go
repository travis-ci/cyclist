package cyclist

import "github.com/Sirupsen/logrus"

func init() {
	if log == nil {
		log = logrus.New()
	}
	log.Level = logrus.FatalLevel
}
