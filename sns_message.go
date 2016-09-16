package cyclist

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
)

var (
	snsSigKeys = []string{
		"Message",
		"MessageID",
		"Subject",
		"Timestamp",
		"TopicARN",
		"Type",
	}
)

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

func (m *snsMessage) verify() error {
	buf := &bytes.Buffer{}
	v := reflect.ValueOf(m)

	for _, key := range snsSigKeys {
		val := reflect.Indirect(v).FieldByName(key).String()
		if val == "" {
			continue
		}
		buf.WriteString(key + "\n")
		buf.WriteString(val + "\n")
	}

	msgSig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return err
	}

	res, err := http.Get(m.SigningCertURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	p, _ := pem.Decode(body)
	cert, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return err
	}

	err = cert.CheckSignature(x509.SHA1WithRSA, buf.Bytes(), msgSig)
	if err != nil {
		return err
	}

	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("unknown public key type %T", cert.PublicKey)
	}

	h := sha1.New()
	n, err := h.Write(buf.Bytes())
	if err != nil {
		return err
	}
	if n != buf.Len() {
		return fmt.Errorf("unexpected number of bytes written expected=%d actual=%d", buf.Len(), n)
	}

	return rsa.VerifyPKCS1v15(pub, crypto.SHA1, h.Sum(nil), msgSig)
}
