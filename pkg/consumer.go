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

	"github.com/flanksource/commons/logger"
	"github.com/flanksource/duty/context"
	"github.com/flanksource/duty/shutdown"
	"github.com/flanksource/gomplate/v3"
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
func RunConsumer(rootCtx context.Context, config Config) error {
	if config.LogLevel != "" {
		logger.StandardLogger().SetLogLevel(config.LogLevel)
		rootCtx.Infof("Set log level to %s => %v", config.LogLevel, rootCtx.Logger.GetLevel())
	}

	if config.client == nil {
		if client, _, err := NewClient(); err != nil {
			return oops.Wrapf(err, "Failed to create Kubernetes client")
		} else {
			config.client = client
		}
	}

	rootCtx.Infof("Config: \n%+v", pretty(config))

	sub, err := config.Subscribe(rootCtx)
	if err != nil {
		return oops.Wrapf(err, "Error building URL")
	}

	rootCtx.Infof("Receiving messages from %+v", sub)
	ctx2, cancel := gocontext.WithCancel(gocontext.Background())
	shutdown.AddHook(func() {
		rootCtx.Infof("Shutting down consumer")
		cancel()
	})

	for {
		msg, err := sub.Receive(ctx2)
		if err != nil {
			if err == gocontext.Canceled || ctx2.Err() == gocontext.Canceled {
				return nil
			}
			rootCtx.Errorf("Error receiving message: %v", err)
			time.Sleep(5 * time.Second)
			continue
		} else if msg == nil {
			rootCtx.Warnf("Queue is empty, waiting for 3 seconds")
			time.Sleep(3 * time.Second)
		}

		ctx := rootCtx.WithName(msg.LoggableID)
		ctx.Logger.SetLogLevel(config.LogLevel)

		// Attempt to decode Bas64
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
			var pod = *config.Pod

			if err := templater.Walk(&pod); err != nil {
				ctx.Errorf("Error templating Pod: %v", err)
				msg.Ack()
				continue
			}

			ctx.Tracef("pod=%s", pretty(pod))

			p, err := config.client.CoreV1().Pods(pod.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
			if p == nil || p.CreationTimestamp.IsZero() {
				p = &pod
			}
			shouldRetry(ctx, msg, p, err)
		} else if config.Job != nil {
			var job = *config.Job

			if err := templater.Walk(&job); err != nil {
				ctx.Errorf("Error templating job: %v", err)
				msg.Ack()
				continue
			}

			ctx.Tracef("job=%s", pretty(job))

			created, err := config.client.BatchV1().Jobs(job.Namespace).Create(ctx, &job, metav1.CreateOptions{})
			if created.CreationTimestamp.IsZero() {
				created = &job
			}

			shouldRetry(ctx, msg, created, err)
		} else {
			return fmt.Errorf("Invalid config, must specify either a job or a pod")
		}
	}

}

func shouldRetry(ctx context.Context, msg *pubsub.Message, accessor metav1.ObjectMetaAccessor, err error) {
	o := accessor.GetObjectMeta()
	name := fmt.Sprintf("%s/%s (uid=%s)", o.GetNamespace(), o.GetName(), o.GetUID())
	if err == nil {
		ctx.Infof("Created %s", name)
		msg.Ack()
		return
	}
	if !IsRetryableError(err) {
		ctx.Errorf("Unretryable error creating: %v\n%s", err, pretty(accessor))
		msg.Ack()
		return
	}
	_delay := time.Second * 5
	if delay, ok := kerrors.SuggestsClientDelay(err); ok {
		_delay = time.Duration(delay)
	}
	if msg.Nackable() {
		msg.Nack()
	}
	ctx.Errorf("Error creating, (retrying in %s %v\n%s", _delay, err, pretty(accessor))
	time.Sleep(_delay)
}
