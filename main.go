package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/gorilla/mux"
	"github.com/meatballhat/negroni-logrus"
	"github.com/urfave/negroni"
)

// SNSMessage is totally an SNS message, eh
type SNSMessage struct {
	Token    string
	TopicARN string `json:"TopicArn"`
}

// http://docs.aws.amazon.com/sns/latest/dg/SendMessageToHttp.html
func handleSNSConfirmation(msg *SNSMessage) error {
	svc := sns.New(session.New(), &aws.Config{Region: aws.String(os.Getenv("SNS_REGION"))})

	params := &sns.ConfirmSubscriptionInput{
		Token:    aws.String(msg.Token),
		TopicArn: aws.String(msg.TopicARN),
	}
	_, err := svc.ConfirmSubscription(params)
	if err != nil {
		return err
	}

	return nil
}

func YourHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello\n")
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", YourHandler)

	n := negroni.New(
		negroni.NewRecovery(),
		negronilogrus.NewMiddleware(),
		negroni.Wrap(r),
	)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := fmt.Sprintf("[::1]:%s", port)
	fmt.Printf("serving on %s\n", addr)
	http.ListenAndServe(addr, n)
}
