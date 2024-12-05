package pkg

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

type Config struct {
	Pod      *corev1.Pod   `json:"pod,omitempty"`
	Job      *batchv1.Job  `json:"job,omitempty"`
	SQS      *SQSConfig    `json:"sqs"`
	PubSub   *PubSubConfig `json:"pubsub"`
	RabbitMQ *RabbitConfig `json:"rabbitmq"`
	Memory   *MemoryConfig `json:"memory"`
	Kafka    *KafkaConfig  `json:"kafka"`
	NATS     *NATSConfig   `json:"nats"`
}

type SQSConfig struct {
	QueueName string `json:"queue"`
	Region    string `json:"region"`
	Account   string `json:"account"`
	Endpoint  string `json:"endpoint"`
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

func (c *Config) BuildURL() (string, error) {
	if c.SQS != nil {
		u := url.URL{
			Scheme: "awssqs",
			Host:   fmt.Sprintf("sqs.%s.amazonaws.com", c.SQS.Region),
			Path:   fmt.Sprintf("%s/%s", c.SQS.Account, c.SQS.QueueName),
		}
		q := u.Query()
		if c.SQS.Region != "" {
			q.Set("region", c.SQS.Region)
		}
		if c.SQS.Endpoint != "" {
			q.Set("endpoint", c.SQS.Endpoint)
			u.RawQuery = q.Encode()
		}
		return u.String(), nil
	}

	if c.PubSub != nil {
		if c.PubSub.ProjectID == "" || c.PubSub.Subscription == "" {
			return "", fmt.Errorf("project_id and subscription are required for GCP Pub/Sub")
		}
		return fmt.Sprintf("gcppubsub://projects/%s/subscriptions/%s", c.PubSub.ProjectID, c.PubSub.Subscription), nil
	}
	if c.Kafka != nil {
		os.Setenv("KAFKA_BROKERS", strings.Join(c.Kafka.Brokers, ","))
		return fmt.Sprintf("kafka://%s?topic=%s", c.Kafka.Group, c.Kafka.Topic), nil
	}

	if c.RabbitMQ != nil {
		os.Setenv("RABBIT_SERVER_URL", c.RabbitMQ.URL.String())
		return fmt.Sprintf("rabbit://%s", c.RabbitMQ.Queue), nil
	}

	if c.NATS != nil {
		os.Setenv("NATS_SERVER_URL", c.NATS.URL.String())
		return fmt.Sprintf("nats://%s", c.NATS.Queue), nil
	}

	if c.Memory != nil {
		return fmt.Sprintf("mem://%s", c.Memory.QueueName), nil
	}

	return "", fmt.Errorf("no queue configuration provided")
}
