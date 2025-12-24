package pkg_test

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
	"github.com/flanksource/batch-runner/pkg"
	batchv1alpha1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	"github.com/flanksource/commons/logger"
	dutyctx "github.com/flanksource/duty/context"
	dutyKubernetes "github.com/flanksource/duty/kubernetes"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	testEnv   *envtest.Environment
	client    *kubernetes.Clientset
	snsClient *sns.Client
	sqsClient *sqs.Client
	topicArn  string
	queueURL  string
)

func TestPkg(t *testing.T) {
	t.Skip("Not required, cover by e2e test")
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pkg Suite")
}

var _ = BeforeSuite(func() {
	testEnv = &envtest.Environment{}
	restConfig, err := testEnv.Start()
	Expect(err).To(BeNil())

	endpoint := "http://localhost:4566"
	region := "us-east-1"

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

	snsClient = sns.NewFromConfig(cfg)
	sqsClient = sqs.NewFromConfig(cfg)

	queueName := "test-batch-runner-" + time.Now().Format("150405")

	topicResult, err := snsClient.CreateTopic(context.TODO(), &sns.CreateTopicInput{
		Name: aws.String(queueName),
	})
	Expect(err).To(BeNil())
	topicArn = *topicResult.TopicArn

	logger.Infof("Created SNS Topic %s", topicArn)
	queueResult, err := sqsClient.CreateQueue(context.TODO(), &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	})
	Expect(err).To(BeNil())
	queueURL = *queueResult.QueueUrl

	queueAttrs, err := sqsClient.GetQueueAttributes(context.TODO(), &sqs.GetQueueAttributesInput{
		QueueUrl:       &queueURL,
		AttributeNames: []types.QueueAttributeName{"QueueArn"},
	})
	Expect(err).To(BeNil())
	queueArn := queueAttrs.Attributes["QueueArn"]

	_, err = snsClient.Subscribe(context.TODO(), &sns.SubscribeInput{
		TopicArn: &topicArn,
		Protocol: aws.String("sqs"),
		Endpoint: &queueArn,
	})
	Expect(err).To(BeNil())

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

	var batchConfig batchv1alpha1.Config
	Expect(yaml.Unmarshal(configData, &batchConfig)).To(BeNil())
	batchConfig.SQS.QueueArn = queueArn
	batchConfig.SQS.AccessKey.ValueStatic = "test"
	batchConfig.SQS.SecretKey.ValueStatic = "test"
	batchConfig.SQS.Endpoint = endpoint
	batchConfig.SQS.WaitTime = 3

	client = kubernetes.NewForConfigOrDie(restConfig)

	ctx := dutyctx.New()
	ctx = ctx.WithLocalKubernetes(dutyKubernetes.NewKubeClient(ctx.Logger, client, restConfig))
	go pkg.RunConsumer(ctx, &batchConfig)
})

var _ = AfterSuite(func() {
	if sqsClient != nil && queueURL != "" {
		sqsClient.DeleteQueue(context.TODO(), &sqs.DeleteQueueInput{
			QueueUrl: &queueURL,
		})
	}
	if snsClient != nil && topicArn != "" {
		snsClient.DeleteTopic(context.TODO(), &sns.DeleteTopicInput{
			TopicArn: &topicArn,
		})
	}
	if testEnv != nil {
		testEnv.Stop()
	}
})
