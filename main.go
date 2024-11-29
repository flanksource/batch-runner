package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flanksource/batch-runner/pkg"
	commonsContext "github.com/flanksource/commons/context"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/gomplate/v3"
	"github.com/spf13/cobra"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/awssnssqs"
	_ "gocloud.dev/pubsub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/kafkapubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	_ "gocloud.dev/pubsub/natspubsub"
	_ "gocloud.dev/pubsub/rabbitpubsub"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var rootCmd = &cobra.Command{
	Use:   "queue-consumer",
	Short: "Consumes messages from a queue",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configPath := "config.yaml"
	configData, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config pkg.Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}
	logger.Infof("Config: %+v", logger.Pretty(config))

	url, err := config.BuildURL()
	if err != nil {
		log.Fatalf("Error building URL: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	logger.Infof("Receiving messages from %s", url)

	// Open subscription using URL from config
	subscription, err := pubsub.OpenSubscription(ctx, url)
	if err != nil {
		log.Fatalf("Failed to open subscription: %v", err)
	}

	client, _, err := pkg.NewClient()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Process messages until signal received
	go func() {
		for {
			msg, err := subscription.Receive(ctx)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				logger.Errorf("Error receiving message: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			logger.Infof("Received message: %s: %v", msg.LoggableID)

			ctx := commonsContext.NewContext(context.TODO())
			log := logger.StandardLogger().Named(msg.LoggableID)
			log.SetLogLevel(logger.Trace4)
			ctx.Logger = log

			// Decode base64
			decoded, err := base64.StdEncoding.DecodeString(string(msg.Body))
			if err != nil {
				decoded = msg.Body
			}

			// Unmarshal to map
			var data map[string]any
			if err := json.Unmarshal(decoded, &data); err != nil {
				logger.Errorf("Error unmarshaling JSON: %v", err)
				msg.Ack()
				continue
			}

			ctx.Infof(logger.Pretty(data))

			data["msg"] = msg

			templater := gomplate.StructTemplater{

				Values:         data,
				DelimSets:      []gomplate.Delims{{Left: "{{", Right: "}}"}},
				ValueFunctions: true,
			}

			var pod = config.Pod

			// Walk the Pod and apply the template to each string value

			if err := templater.Walk(&pod); err != nil {
				logger.Errorf("Error templating Pod: %v", err)
				msg.Ack()
				continue
			}

			// // remarshal the pod to v1 type
			// var podv1 corev1.Pod
			// podJSON, err := json.Marshal(pod)

			// if err := json.Unmarshal(podJSON, &podv1); err != nil {
			// 	logger.Errorf("Error unmarshaling Pod: %v", err)
			// 	msg.Ack()
			// 	continue
			// }

			ctx.Infof(logger.Pretty(pod))

			p, err := client.CoreV1().Pods(pod.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
			ctx.Infof("Created %s", p.UID)

			if err != nil {
				ctx.Infof("Error creating Pod: %v", err)
				msg.Ack()
			}

		}
	}()
	// Wait for signal
	<-sigChan
	fmt.Println("\nShutting down...")
}

func main() {
	logger.BindFlags(rootCmd.Flags())
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
