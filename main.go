package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/flanksource/batch-runner/pkg"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/duty"
	"github.com/flanksource/duty/shutdown"
	"github.com/spf13/cobra"
	_ "gocloud.dev/pubsub/awssnssqs"
	_ "gocloud.dev/pubsub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	_ "gocloud.dev/pubsub/kafkapubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	_ "gocloud.dev/pubsub/natspubsub"
	_ "gocloud.dev/pubsub/rabbitpubsub"
	"sigs.k8s.io/yaml"

	batchv1alpha1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
)

var rootCmd = &cobra.Command{
	Use:   "queue-consumer",
	Short: "Consumes messages from a queue",
	Args:  cobra.MinimumNArgs(0),
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	ctx, cancel, err := duty.Start("batch-runner", duty.ClientOnly)
	defer cancel()
	if err != nil {
		logger.Fatalf("Error starting duty: %v", err)
		os.Exit(1)
	}

	shutdown.WaitForSignal()

	configFiles = append(configFiles, args...)

	logger.Infof("Starting consumer with config files: %v", configFiles)

	wg := sync.WaitGroup{}

	for _, configFile := range configFiles {
		if configFile == "" {
			continue
		}
		configData, err := os.ReadFile(configFile)
		if err != nil {
			logger.Fatalf("Error reading config file: %v", err)
		}

		var config batchv1alpha1.Config
		if err := yaml.Unmarshal(configData, &config); err != nil {
			logger.Fatalf("Error parsing config file: %v", err)
			os.Exit(1)
		}

		wg.Add(1)

		go func() {
			if err := pkg.RunConsumer(ctx, &config); err != nil {
				logger.Errorf("Error running consumer: %v", err)
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

var configFiles []string

func main() {
	rootCmd.Flags().StringArrayVarP(&configFiles, "config", "c", []string{}, "Path to config file")
	rootCmd.Flags().MarkDeprecated("config", "Pass the config files as arguments instead")
	logger.BindFlags(rootCmd.Flags())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
