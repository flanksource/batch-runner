package main

import (
	"fmt"
	"log"
	"os"

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
)

var rootCmd = &cobra.Command{
	Use:   "queue-consumer",
	Short: "Consumes messages from a queue",
	Run:   run,
}

func run(cmd *cobra.Command, args []string) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config pkg.Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}

	ctx, cancel, err := duty.Start("batch-runner", duty.ClientOnly)

	if err != nil {
		log.Fatalf("Error starting duty: %v", err)
		os.Exit(1)
	}
	shutdown.WaitForSignal()

	if err := pkg.RunConsumer(ctx, config); err != nil {
		logger.Fatalf("Error running consumer: %v", err)
		cancel()
	}

}

var configPath string

func main() {
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to config file")
	logger.BindFlags(rootCmd.Flags())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
