package cyclist

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

// snsMessage is totally an SNS message, eh
type snsMessage struct {
	Token    string
	TopicARN string `json:"TopicArn"`
}

func handleSNSConfirmation(msg *snsMessage) error {
	svc := sns.New(session.New(), &aws.Config{Region: aws.String(os.Getenv("SNS_REGION"))})

	params := &sns.ConfirmSubscriptionInput{
		Token:    aws.String(msg.Token),
		TopicArn: aws.String(msg.TopicARN),
	}
	_, err := svc.ConfirmSubscription(params)
	return err
}

func snsHandler(w http.ResponseWriter, r *http.Request) {
	msg := &snsMessage{}
	err := json.NewDecoder(r.Body).Decode(msg)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "nope\n")
		return
	}

	err = handleSNSConfirmation(msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "wow sorry: %s\n", err)
		return
	}
}
