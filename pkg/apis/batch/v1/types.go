package v1

import (
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/flanksource/duty/connection"
	dutyps "github.com/flanksource/duty/pubsub"
	"github.com/flanksource/duty/shell"
	"github.com/flanksource/duty/types"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=batch
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Queue",type=string,JSONPath=`.status.connectionState`
// +kubebuilder:printcolumn:name="Processed",type=integer,JSONPath=`.status.messagesProcessed`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.messagesFailed`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// BatchTrigger is the Schema for the batch-runner configuration
type BatchTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Config             `json:"spec,omitempty"`
	Status BatchTriggerStatus `json:"status,omitempty"`
}

// BatchTriggerStatus defines the observed state of BatchTrigger
type BatchTriggerStatus struct {
	// ConnectionState indicates queue connection status: Connected, Disconnected, Error
	ConnectionState string `json:"connectionState,omitempty"`

	// MessagesProcessed is the total number of successfully processed messages
	MessagesProcessed int64 `json:"messagesProcessed,omitempty"`

	// MessagesFailed is the total number of failed messages
	MessagesFailed int64 `json:"messagesFailed,omitempty"`

	// MessagesRetried is the total number of retried messages
	MessagesRetried int64 `json:"messagesRetried,omitempty"`

	// LastError contains the most recent error message
	LastError string `json:"lastError,omitempty"`

	// LastErrorTime is when the last error occurred
	LastErrorTime *metav1.Time `json:"lastErrorTime,omitempty"`

	// Conditions represent the latest available observations of the BatchTrigger's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// BatchTriggerList contains a list of BatchTrigger
type BatchTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BatchTrigger `json:"items"`
}

// Config defines the desired state of BatchTrigger
// +kubebuilder:object:generate=true
type Config struct {
	// +optional
	LogLevel string `json:"logLevel,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Pod *corev1.Pod `json:"pod,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Job *batchv1.Job `json:"job,omitempty"`
	Exec               *ExecAction  `json:"exec,omitempty"`
	dutyps.QueueConfig `json:",inline"`
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
	Attempts int `json:"attempts,omitempty"`
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
