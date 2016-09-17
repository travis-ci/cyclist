package cyclist

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"net/http"
	"reflect"

	"github.com/pkg/errors"
)

var (
	// NOTE: this declaration is broken up mostly to appease the gofmt checker,
	//       although I don't know why ~meatballhat
	snsSigKeys      = map[string][]string{}
	snsKeyRealNames = map[string]string{
		"MessageID": "MessageId",
		"TopicARN":  "TopicArn",
	}

	errEmptyPEM = errors.New("nothing found in pem encoded bytes")
)

func init() {
	snsSigKeys["Notification"] = []string{
		"Message",
		"MessageID",
		"Subject",
		"Timestamp",
		"TopicARN",
		"Type",
	}
	snsSigKeys["SubscriptionConfirmation"] = []string{
		"Message",
		"MessageID",
		"SubscribeURL",
		"Timestamp",
		"Token",
		"TopicARN",
		"Type",
	}
}

type snsMessage struct {
	Message        string
	MessageID      string `json:"MessageId"`
	Token          string
	TopicARN       string `json:"TopicArn"`
	Type           string
	Subject        string
	Timestamp      string
	Signature      string
	SigningCertURL string
}

func (m *snsMessage) lifecycleAction() (*lifecycleAction, error) {
	a := &lifecycleAction{}
	err := json.Unmarshal([]byte(m.Message), a)
	if err != nil {
		return nil, err
	}

	return a, nil
}

func (m *snsMessage) sigSerialized() []byte {
	buf := &bytes.Buffer{}
	v := reflect.ValueOf(m)

	for _, key := range snsSigKeys[m.Type] {
		field := reflect.Indirect(v).FieldByName(key)
		val := field.String()
		if !field.IsValid() || val == "" {
			continue
		}
		if rn, ok := snsKeyRealNames[key]; ok {
			key = rn
		}
		buf.WriteString(key + "\n")
		buf.WriteString(val + "\n")
	}

	return buf.Bytes()
}

func (m *snsMessage) verify() error {
	msgSig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return errors.Wrap(err, "failed to base64 decode signature")
	}

	res, err := http.Get(m.SigningCertURL)
	if err != nil {
		return errors.Wrap(err, "failed to fetch signing cert")
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read signing cert body")
	}

	p, _ := pem.Decode(body)
	if p == nil {
		return errEmptyPEM
	}

	cert, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return errors.Wrap(err, "failed to parse signing cert")
	}

	err = cert.CheckSignature(x509.SHA1WithRSA, m.sigSerialized(), msgSig)
	if err != nil {
		return errors.Wrap(err, "message signature check error")
	}

	return nil
}
