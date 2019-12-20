package main

import (
	"fmt"
	"log"
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

	eventParam := &health.DescribeEventsInput{
		Filter: &health.EventFilter{
			EventStatusCodes: []*string{
				aws.String(health.EventStatusCodeOpen),
				aws.String(health.EventStatusCodeUpcoming),
			},
			EventTypeCategories: []*string{
				aws.String(health.EventTypeCategoryScheduledChange),
			},
		},
	}

	for _, target := range req.TargetList {
		creds := stscreds.NewCredentialsWithClient(assumeRoler, target.Role)
		svc := health.New(sess, aws.NewConfig().WithRegion("us-east-1").WithCredentials(creds))

		var arns []*string
		err := svc.DescribeEventsPages(eventParam, func(resp *health.DescribeEventsOutput, lastPage bool) bool {
			for _, event := range resp.Events {
				arns = append(arns, event.Arn)
			}
			return true
		})

		if err != nil {
			log.Println(err.Error())
			continue
		}

		entityParam := &health.DescribeAffectedEntitiesInput{
			Filter: &health.EntityFilter{
				EventArns: arns,
			},
		}

		var entities []*health.AffectedEntity
		err = svc.DescribeAffectedEntitiesPages(entityParam, func(resp *health.DescribeAffectedEntitiesOutput, lastPage bool) bool {
			entities = append(entities, resp.Entities...)
			return true
		})

		if err != nil {
			log.Println(err.Error())
			continue
		}

		entities = removeUnknown(entities)

		err = client.PostServiceMetricValues(target.Service, []*mackerel.MetricValue{
			&mackerel.MetricValue{
				Name:  "monitor-maintenance." + target.Name,
				Time:  nowTime.Unix(),
				Value: len(entities),
			},
		})
		if err != nil {
			fmt.Println(err.Error())
			continue
		}
	}
	return "ok", nil
}

func removeUnknown(e []*health.AffectedEntity) []*health.AffectedEntity {
	result := []*health.AffectedEntity{}
	for _, v := range e {
		if *v.EntityValue == health.EntityStatusCodeUnknown {
			continue
		}
		result = append(result, v)
	}
	return result
}

func main() {
	lambda.Start(handler)
}
