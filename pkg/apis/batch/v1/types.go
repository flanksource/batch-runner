package v1

import (
	"fmt"
	"net/url"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/flanksource/duty/connection"
	"github.com/flanksource/duty/shell"
	"github.com/flanksource/duty/types"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=batch
// BatchTrigger is the Schema for the batch-runner configuration
type BatchTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec Config `json:"spec,omitempty"`
}

// Config defines the desired state of BatchTrigger
// +kubebuilder:object:generate=true
type Config struct {
	// +optional
	LogLevel string        `json:"logLevel,omitempty"`
	Pod      *corev1.Pod   `json:"pod,omitempty"`
	Job      *batchv1.Job  `json:"job,omitempty"`
	Exec     *ExecAction   `json:"exec,omitempty"`
	SQS      *SQSConfig    `json:"sqs,omitempty"`
	PubSub   *PubSubConfig `json:"pubsub,omitempty"`
	RabbitMQ *RabbitConfig `json:"rabbitmq,omitempty"`
	Memory   *MemoryConfig `json:"memory,omitempty"`
	Kafka    *KafkaConfig  `json:"kafka,omitempty"`
	NATS     *NATSConfig   `json:"nats,omitempty"`
}

func (c *Config) GetQueue() fmt.Stringer {
	if c.SQS != nil {
		return *c.SQS
	}
	if c.PubSub != nil {
		return *c.PubSub
	}
	if c.RabbitMQ != nil {
		return *c.RabbitMQ
	}
	if c.Memory != nil {
		return *c.Memory
	}
	if c.Kafka != nil {
		return *c.Kafka
	}
	if c.NATS != nil {
		return *c.NATS
	}
	return nil
}

type S string

func (s S) String() string {
	return string(s)
}

func (c *Config) GetDestination() fmt.Stringer {
	if c.Job != nil {
		return S(fmt.Sprintf("%s/%s", c.Job.Namespace, c.Job.Name))
	}
	if c.Pod != nil {
		return S(fmt.Sprintf("%s/%s", c.Pod.Namespace, c.Pod.Name))
	}
	if c.Exec != nil {
		return c.Exec
	}
	return nil
}

func (c Config) String() string {
	dest := c.GetDestination()

	queue := c.GetQueue()

	return fmt.Sprintf("%s -> %s", queue, dest)
}

type ExecAction struct {
	// Script can be an inline script or a path to a script that needs to be executed
	// On windows executed via powershell and in darwin and linux executed using bash
	Script      string                     `yaml:"script" json:"script" template:"true"`
	Connections connection.ExecConnections `yaml:"connections,omitempty" json:"connections,omitempty" template:"true"`
	// Artifacts to save
	Artifacts []shell.Artifact `yaml:"artifacts,omitempty" json:"artifacts,omitempty" template:"true"`
	// EnvVars are the environment variables that are accessible to exec processes
	EnvVars []types.EnvVar `yaml:"env,omitempty" json:"env,omitempty"`
	// Checkout details the git repository that should be mounted to the process
	Checkout *connection.GitConnection `yaml:"checkout,omitempty" json:"checkout,omitempty"`

	Retry *Retry `yaml:"retry,omitempty" json:"retry,omitempty"`
}

type Retry struct {
	Attempts int `json:"attempts"`
	// Delay is the time in seconds to wait between retries
	Delay int `json:"delay"`
}

func (r Retry) next(attempts int) time.Duration {
	return time.Second * time.Duration(r.Delay)
}

func (e ExecAction) String() string {
	return e.Script
}

func (e *ExecAction) ToShellExec() shell.Exec {
	return shell.Exec{
		Script:      e.Script,
		Connections: e.Connections,
		EnvVars:     e.EnvVars,
		Artifacts:   e.Artifacts,
		Checkout:    e.Checkout,
	}
}

// +kubebuilder:object:generate=true
type SQSConfig struct {
	QueueArn    string `json:"queue"`
	RawDelivery bool   `json:"raw"`
	// Time in seconds to long-poll for messages, Default to 15, max is 20
	WaitTime                 int `json:"waitTime,omitempty"`
	connection.AWSConnection `json:",inline"`
}

func (s SQSConfig) String() string {
	return s.QueueArn
}

// +kubebuilder:object:generate=true
type KafkaConfig struct {
	Brokers []string `json:"brokers"`
	Topic   string   `json:"topic"`
	Group   string   `json:"group"`
}

func (k KafkaConfig) String() string {
	return fmt.Sprintf("kafka://%s", k.Topic)
}

// +kubebuilder:object:generate=true
type PubSubConfig struct {
	ProjectID    string `json:"project_id"`
	Subscription string `json:"subscription"`
}

func (p PubSubConfig) String() string {
	return fmt.Sprintf("gcppubsub://projects/%s/subscriptions/%s", p.ProjectID, p.Subscription)
}

// +kubebuilder:object:generate=true
type NATSConfig struct {
	URL     `json:",inline"`
	Subject string `json:"subject"`
	Queue   string `json:"queue"`
}

func (n NATSConfig) String() string {
	return fmt.Sprintf("nats://%s", n.Queue)
}

// +kubebuilder:object:generate=true
type RabbitConfig struct {
	URL   `json:",inline"`
	Queue string `json:"queue"`
}

func (r RabbitConfig) String() string {
	return fmt.Sprintf("rabbit://%s", r.Queue)
}

// +kubebuilder:object:generate=true
type MemoryConfig struct {
	QueueName string `json:"queue"`
}

func (m MemoryConfig) String() string {
	return fmt.Sprintf("mem://%s", m.QueueName)
}

// +kubebuilder:object:generate=true
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
