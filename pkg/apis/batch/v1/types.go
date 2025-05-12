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
	LogLevel           string       `json:"logLevel,omitempty"`
	Pod                *corev1.Pod  `json:"pod,omitempty"`
	Job                *batchv1.Job `json:"job,omitempty"`
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
