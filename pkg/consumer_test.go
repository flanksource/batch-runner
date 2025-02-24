package pkg

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/flanksource/duty"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

func TestSNSToSQSIntegration(t *testing.T) {
	RegisterTestingT(t)

	testEnv := &envtest.Environment{}

	// start testEnv
	restConfig, err := testEnv.Start()
	Expect(err).To(BeNil())

	defer testEnv.Stop()

	// LocalStack configuration
	endpoint := "http://localhost:4566"
	region := "us-east-1"

	// Create AWS config for LocalStack
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithEndpointResolver(aws.EndpointResolverFunc(
			func(service, region string) (aws.Endpoint, error) {
				return aws.Endpoint{URL: endpoint}, nil
			})),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "test",
				SecretAccessKey: "test",
				SessionToken:    "test",
			},
		}),
	)
	Expect(err).To(BeNil())

	// Create SNS and SQS clients
	snsClient := sns.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)

	queueName := "test-batch-runner-" + time.Now().Format("150405")

	// Create SNS topic
	topicResult, err := snsClient.CreateTopic(context.TODO(), &sns.CreateTopicInput{
		Name: aws.String(queueName),
	})
	Expect(err).To(BeNil())
	topicArn := *topicResult.TopicArn

	// Create SQS queue
	queueResult, err := sqsClient.CreateQueue(context.TODO(), &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})

	Expect(err).To(BeNil())
	queueURL := *queueResult.QueueUrl

	// Get queue ARN
	queueAttrs, err := sqsClient.GetQueueAttributes(context.TODO(), &sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURL,
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	Expect(err).To(BeNil())
	queueArn := queueAttrs.Attributes["QueueArn"]

	// Subscribe queue to topic
	_, err = snsClient.Subscribe(context.TODO(), &sns.SubscribeInput{
		TopicArn: &topicArn,
		Protocol: aws.String("sqs"),
		Endpoint: &queueArn,
	})
	Expect(err).To(BeNil())

	// Set queue policy to allow SNS
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": "*",
			"Action": "sqs:SendMessage",
			"Resource": "%s",
			"Condition": {
				"ArnEquals": {
					"aws:SourceArn": "%s"
				}
			}
		}]
	}`, queueArn, topicArn)

	_, err = sqsClient.SetQueueAttributes(context.TODO(), &sqs.SetQueueAttributesInput{
		QueueUrl: &queueURL,
		Attributes: map[string]string{
			"Policy": policy,
		},
	})
	Expect(err).To(BeNil())

	configData, err := os.ReadFile("../config-pod.yaml")
	Expect(err).To(BeNil())

	var config Config
	Expect(yaml.Unmarshal(configData, &config)).To(BeNil())
	config.SQS.QueueArn = queueArn
	config.SQS.AccessKey.ValueStatic = "test"
	config.SQS.SecretKey.ValueStatic = "test"
	config.SQS.Endpoint = endpoint
	config.SQS.WaitTime = 3

	config.client = kubernetes.NewForConfigOrDie(restConfig)

	ctx, cancel, err := duty.Start("batch-runner", duty.ClientOnly)
	Expect(err).To(BeNil())
	defer cancel()
	go RunConsumer(ctx, config)
	// Publish message to SNS
	testMessage := "{\"a\": \"b\"}"
	_, err = snsClient.Publish(context.TODO(), &sns.PublishInput{
		TopicArn: &topicArn,
		Message:  &testMessage,
	})
	Expect(err).To(BeNil())

	findPod := func() *corev1.Pod {
		if pod, e := config.client.CoreV1().Pods("default").Get(context.TODO(), "batch-b", v1.GetOptions{}); e == nil {
			return pod
		}
		return nil
	}

	Eventually(findPod).
		WithTimeout(10 * time.Second).
		WithPolling(time.Second).
		ShouldNot(BeNil())
	pod := findPod()
	Expect(pod.Name).To(Equal("batch-b"))

	// Cleanup

	_, err = sqsClient.DeleteQueue(context.TODO(), &sqs.DeleteQueueInput{
		QueueUrl: &queueURL,
	})
	Expect(err).To(BeNil())

	_, err = snsClient.DeleteTopic(context.TODO(), &sns.DeleteTopicInput{
		TopicArn: &topicArn,
	})
	Expect(err).To(BeNil())
}
