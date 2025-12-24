package pkg

import (
	gocontext "context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/duty/context"
	dutyps "github.com/flanksource/duty/pubsub"
	"github.com/flanksource/duty/shell"
	"github.com/flanksource/gomplate/v3"
	"github.com/samber/lo"
	"github.com/samber/oops"

	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"
	_ "gocloud.dev/pubsub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/kafkapubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	_ "gocloud.dev/pubsub/natspubsub"
	_ "gocloud.dev/pubsub/rabbitpubsub"
)

var retry = NewRetryCache()

type ConsumerCallbacks struct {
	OnMessageProcessed func()
	OnMessageFailed    func(err error)
	OnMessageRetried   func()
	OnConnectionChange func(state string)
}

func pretty(o any) string {
	s, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err.Error()
	}

	b, err := yaml.JSONToYAML(s)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
func RunConsumer(rootCtx context.Context, config *v1.Config) error {
	return RunConsumerWithCallbacks(rootCtx, config, nil)
}

func RunConsumerWithCallbacks(rootCtx context.Context, config *v1.Config, callbacks *ConsumerCallbacks) error {
	if config.LogLevel != "" {
		logger.StandardLogger().SetLogLevel(config.LogLevel)
		rootCtx.Infof("Set log level to %s => %v", config.LogLevel, rootCtx.Logger.GetLevel())
	}

	rootCtx.Tracef("Config: \n%+v", pretty(config))

	sub, err := dutyps.Subscribe(rootCtx, config.QueueConfig)
	if err != nil {
		if callbacks != nil && callbacks.OnConnectionChange != nil {
			callbacks.OnConnectionChange("Error")
		}
		return oops.Wrapf(err, "Error building URL")
	}

	if callbacks != nil && callbacks.OnConnectionChange != nil {
		callbacks.OnConnectionChange("Connected")
	}

	rootCtx.Infof("Consuming from %s", config.String())

	for {
		if rootCtx.Err() != nil {
			return rootCtx.Err()
		}

		msg, err := sub.Receive(rootCtx)
		if err != nil {
			if err == gocontext.Canceled || rootCtx.Err() != nil {
				return nil
			}
			rootCtx.Errorf("Error receiving message: %v", err)
			time.Sleep(5 * time.Second)
			continue
		} else if msg == nil {
			rootCtx.Warnf("Queue is empty, waiting for 3 seconds")
			time.Sleep(3 * time.Second)
			continue
		}

		ctx := rootCtx.WithName(lo.CoalesceOrEmpty(msg.LoggableID, "unknown"))
		ctx.Logger.SetLogLevel(config.LogLevel)

		client, err := ctx.LocalKubernetes()
		if err != nil {
			return oops.Wrapf(err, "Error getting Kubernetes client")
		}

		// Attempt to decode Base64
		decoded, err := base64.StdEncoding.DecodeString(string(msg.Body))
		if err != nil {
			decoded = msg.Body
		}

		// Attempt to unmarshal to map
		var data map[string]any
		if err := json.Unmarshal(decoded, &data); err != nil {
			ctx.Tracef("Error unmarshalling message to json: %v", err)
			data = map[string]any{"body": string(decoded)}
		}
		data["_raw_body"] = string(msg.Body)
		data["_id"] = msg.LoggableID
		data["_metadata"] = msg.Metadata

		ctx.Debugf("Received message:\n %+v", pretty(data))

		templater := gomplate.StructTemplater{
			Values:         data,
			DelimSets:      []gomplate.Delims{{Left: "{{", Right: "}}"}},
			ValueFunctions: true,
		}

		if config.Pod != nil {
			var pod = config.Pod.DeepCopy()

			if err := templater.Walk(&pod); err != nil {
				ctx.Errorf("Error templating Pod: %v", err)
				if callbacks != nil && callbacks.OnMessageFailed != nil {
					callbacks.OnMessageFailed(err)
				}
				msg.Ack()
				continue
			}

			ctx.Tracef("pod=%s", pretty(pod))

			p, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			if p == nil || p.CreationTimestamp.IsZero() {
				p = pod
			}
			shouldRetryWithCallbacks(ctx, msg, p, err, callbacks)
		} else if config.Job != nil {
			var job = config.Job.DeepCopy()

			if err := templater.Walk(job); err != nil {
				ctx.Errorf("Error templating job: %v", err)
				if callbacks != nil && callbacks.OnMessageFailed != nil {
					callbacks.OnMessageFailed(err)
				}
				msg.Ack()
				continue
			}

			ctx.Tracef("job=%s", pretty(job))

			created, err := client.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
			if created.CreationTimestamp.IsZero() {
				created = job
			}

			shouldRetryWithCallbacks(ctx, msg, created, err, callbacks)
		} else if config.Exec != nil {
			exec := *config.Exec
			if err := templater.Walk(&exec); err != nil {
				ctx.Errorf("Error templating exec: %v", err)
				if callbacks != nil && callbacks.OnMessageFailed != nil {
					callbacks.OnMessageFailed(err)
				}
				msg.Ack()
				continue
			}

			ctx.Tracef("job=%s", pretty(exec))

			details, err := shell.Run(ctx, exec.ToShellExec())
			if err == nil && details.ExitCode == 0 {
				ctx.Tracef("%s", details.String())
				if callbacks != nil && callbacks.OnMessageProcessed != nil {
					callbacks.OnMessageProcessed()
				}
				msg.Ack()
				continue
			}

			if exec.Retry == nil {
				exec.Retry = &v1.Retry{
					Attempts: 3,
					Delay:    30,
				}
			}

			execErr := err
			if execErr == nil {
				execErr = fmt.Errorf("script returned non-zero exit code: %s", details)
			}

			if err != nil {
				ctx.Errorf("Error running %s: %s\n%s", exec.Script, err, details)
			} else {
				ctx.Errorf("Script returned non-zero exit code: %s", details)
			}

			delay := retry.GetBackoff(ctx, msg.LoggableID, exec.Retry)
			if delay != nil {
				if callbacks != nil && callbacks.OnMessageRetried != nil {
					callbacks.OnMessageRetried()
				}
				if msg.Nackable() {
					msg.Nack()
				}
				time.Sleep(*delay)
			} else {
				if callbacks != nil && callbacks.OnMessageFailed != nil {
					callbacks.OnMessageFailed(execErr)
				}
				msg.Ack()
			}

		} else {
			return fmt.Errorf("Invalid config, must specify either a job or a pod")
		}
	}
}

func shouldRetry(ctx context.Context, msg *pubsub.Message, accessor metav1.ObjectMetaAccessor, err error) {
	shouldRetryWithCallbacks(ctx, msg, accessor, err, nil)
}

func shouldRetryWithCallbacks(ctx context.Context, msg *pubsub.Message, accessor metav1.ObjectMetaAccessor, err error, callbacks *ConsumerCallbacks) {
	o := accessor.GetObjectMeta()
	name := fmt.Sprintf("%s/%s (uid=%s)", o.GetNamespace(), o.GetName(), o.GetUID())
	if err == nil {
		ctx.Infof("Created %s", name)
		if callbacks != nil && callbacks.OnMessageProcessed != nil {
			callbacks.OnMessageProcessed()
		}
		msg.Ack()
		return
	}
	if !IsRetryableError(err) {
		ctx.Errorf("Unretryable error creating: %v\n%s", err, pretty(accessor))
		if callbacks != nil && callbacks.OnMessageFailed != nil {
			callbacks.OnMessageFailed(err)
		}
		msg.Ack()
		return
	}
	_delay := time.Second * 5
	if delay, ok := kerrors.SuggestsClientDelay(err); ok {
		_delay = time.Duration(delay)
	}
	if callbacks != nil && callbacks.OnMessageRetried != nil {
		callbacks.OnMessageRetried()
	}
	if msg.Nackable() {
		msg.Nack()
	}
	ctx.Errorf("Error creating, (retrying in %s %v\n%s", _delay, err, pretty(accessor))
	time.Sleep(_delay)
}
