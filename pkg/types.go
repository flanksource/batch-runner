package pkg

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/duty/connection"
	"github.com/flanksource/duty/context"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/awssnssqs"
	"gocloud.dev/pubsub/kafkapubsub"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type Config struct {
	LogLevel string        `json:"logLevel,omitempty"`
	Pod      *corev1.Pod   `json:"pod,omitempty"`
	Job      *batchv1.Job  `json:"job,omitempty"`
	SQS      *SQSConfig    `json:"sqs,omitempty"`
	PubSub   *PubSubConfig `json:"pubsub,omitempty"`
	RabbitMQ *RabbitConfig `json:"rabbitmq,omitempty"`
	Memory   *MemoryConfig `json:"memory,omitempty"`
	Kafka    *KafkaConfig  `json:"kafka,omitempty"`
	NATS     *NATSConfig   `json:"nats,omitempty"`

	client kubernetes.Interface
}

type SQSConfig struct {
	QueueArn    string `json:"queue"`
	RawDelivery bool   `json:"raw"`
	// Time in seconds to long-poll for messages, Default to 15, max is 20
	WaitTime                 int `json:"waitTime,omitemptu"`
	connection.AWSConnection `json:",inline"`
}
type KafkaConfig struct {
	Brokers []string `json:"brokers"`
	Topic   string   `json:"topic"`
	Group   string   `json:"group"`
}

type PubSubConfig struct {
	ProjectID    string `json:"project_id"`
	Subscription string `json:"subscription"`
}
type NATSConfig struct {
	URL     `json:",inline"`
	Subject string `json:"subject"`
	Queue   string `json:"queue"`
}

type RabbitConfig struct {
	URL   `json:",inline"`
	Queue string `json:"queue"`
}
type MemoryConfig struct {
	QueueName string `json:"queue"`
}

type URL struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (u URL) String() string {
	_url := url.URL{
		Host: u.Host,
	}
	if u.Username != "" {
		_url.User = url.UserPassword(u.Username, u.Password)

	}
	return _url.String()
}

func (c *Config) Subscribe(ctx context.Context) (*pubsub.Subscription, error) {
	if c.SQS != nil {

		if c.SQS.WaitTime == 0 {
			c.SQS.WaitTime = 5
		}
		c.SQS.AWSConnection.Populate(ctx)
		ctx = ctx.WithName("aws")
		ctx.Logger.SetMinLogLevel(logger.Trace)
		ctx.Logger.SetLogLevel(logger.Info)
		sess, err := c.SQS.AWSConnection.Client(ctx)
		if err != nil {
			return nil, err
		}
		arn, err := ParseArn(c.SQS.QueueArn)
		if err != nil {
			return nil, err
		}

		client := sqs.NewFromConfig(sess, func(o *sqs.Options) {
			if c.SQS.Endpoint != "" {
				o.BaseEndpoint = &c.SQS.Endpoint
			}
		})
		ctx.Infof("Connecting to SQS queue: %s", arn.ToQueueURL())

		return awssnssqs.OpenSubscriptionV2(ctx, client, arn.ToQueueURL(), &awssnssqs.SubscriptionOptions{
			Raw:      c.SQS.RawDelivery,
			WaitTime: time.Second * time.Duration(c.SQS.WaitTime),
		}), nil
	}

	if c.PubSub != nil {
		if c.PubSub.ProjectID == "" || c.PubSub.Subscription == "" {
			return nil, fmt.Errorf("project_id and subscription are required for GCP Pub/Sub")
		}
		return pubsub.OpenSubscription(ctx, fmt.Sprintf("gcppubsub://projects/%s/subscriptions/%s", c.PubSub.ProjectID, c.PubSub.Subscription))
	}
	if c.Kafka != nil {
		return kafkapubsub.OpenSubscription(c.Kafka.Brokers, nil, c.Kafka.Group, []string{c.Kafka.Topic}, nil)
	}

	if c.RabbitMQ != nil {
		return pubsub.OpenSubscription(ctx, fmt.Sprintf("rabbit://%s", c.RabbitMQ.Queue))
	}

	if c.NATS != nil {
		os.Setenv("NATS_SERVER_URL", c.NATS.URL.String())
		return pubsub.OpenSubscription(ctx, fmt.Sprintf("nats://%s", c.NATS.Queue))
	}

	if c.Memory != nil {
		return pubsub.OpenSubscription(ctx, fmt.Sprintf("mem://%s", c.Memory.QueueName))
	}

	return nil, fmt.Errorf("no queue configuration provided")
}
