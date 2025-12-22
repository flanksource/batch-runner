package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/flanksource/batch-runner/cmd"
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

func parseConfigFile(configFiles []string) ([]batchv1alpha1.Config, error) {

	var configs []batchv1alpha1.Config

	for _, configFile := range configFiles {
		configData, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading config file %s: %v", configFile, err)
		}

		re := regexp.MustCompile(`(?m)^---\n`)
		for _, chunk := range re.Split(string(configData), -1) {
			if strings.TrimSpace(chunk) == "" {
				continue
			}

			var config batchv1alpha1.Config
			if err := yaml.Unmarshal([]byte(chunk), &config); err != nil {
				return nil, fmt.Errorf("error parsing config file: %w", err)
			}
			configs = append(configs, config)
		}
	}
	return configs, nil
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

	configs, err := parseConfigFile(configFiles)
	if err != nil {
		logger.Fatalf(err.Error())
		os.Exit(1)
	}
	for _, config := range configs {

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
	_ = rootCmd.Flags().MarkDeprecated("config", "Pass the config files as arguments instead")
	logger.BindFlags(rootCmd.Flags())

	rootCmd.AddCommand(cmd.ControllerCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
