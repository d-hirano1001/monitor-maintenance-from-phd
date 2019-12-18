package main

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/health"
	"github.com/aws/aws-sdk-go/service/sts"
	mackerel "github.com/mackerelio/mackerel-client-go"
)

var (
	client  = mackerel.NewClient(os.Getenv("APIKEY"))
	nowTime = time.Now()
)

// Request is argument specified at call
type Request struct {
	TargetList []TargetAccount `json:"TargetList"`
}

// TargetAccount is target account
type TargetAccount struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	Role    string `json:"Role"`
}

func handler(req Request) (string, error) {

	sess := session.Must(session.NewSession())
	assumeRoler := sts.New(sess)

	filter := &health.EventFilter{
		EventStatusCodes: []*string{
			aws.String(health.EventStatusCodeOpen),
			aws.String(health.EventStatusCodeUpcoming),
		},
		EventTypeCategories: []*string{
			aws.String(health.EventTypeCategoryScheduledChange),
		},
	}

	for _, target := range req.TargetList {
		creds := stscreds.NewCredentialsWithClient(assumeRoler, target.Role)
		svc := health.New(sess, aws.NewConfig().WithRegion("us-east-1").WithCredentials(creds))

		result, err := svc.DescribeEvents(&health.DescribeEventsInput{
			Filter: filter,
		})
		if err != nil {
			fmt.Println(err.Error())
			continue
		}

		err = client.PostServiceMetricValues(target.Service, []*mackerel.MetricValue{
			&mackerel.MetricValue{
				Name:  "monitor-maintenance." + target.Name,
				Time:  nowTime.Unix(),
				Value: len(result.Events),
			},
		})
		if err != nil {
			fmt.Println(err.Error())
			continue
		}
	}

	return "ok", nil
}

func main() {
	lambda.Start(handler)
}
