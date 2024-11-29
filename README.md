
# Queue Consumer

A service that consumes messages from various queue implementations and creates Kubernetes pods based on the message content.

## Features

- Supports multiple queue implementations via gocloud.dev/pubsub:
  - AWS SNS/SQS
  - Azure Service Bus
  - Google Cloud Pub/Sub
  - Kafka
  - NATS
  - RabbitMQ
  - In-memory (for testing)

- Message processing:
  - Base64 decode support
  - JSON message parsing
  - Template-based pod creation using gomplate
  - Kubernetes pod creation from templates

## Configuration

Create a `config.yaml` file with the following structure:

 ```yaml
 pod: # Kubernetes pod configuration
   # Any valid pod spec fields

 sqs: # AWS SQS configuration
   queue: string    # Queue name
   region: string   # AWS region
   account: string  # AWS account ID
   endpoint: string # Optional endpoint URL

 pubsub: # Google Cloud Pub/Sub configuration
   project_id: string    # GCP project ID
   subscription: string  # Pub/Sub subscription name

 kafka: # Apache Kafka configuration
   brokers: [string]  # List of Kafka brokers
   topic: string      # Kafka topic name
   group: string      # Consumer group ID

 rabbitmq: # RabbitMQ configuration
   host: string     # RabbitMQ host
   port: int        # RabbitMQ port
   username: string # Optional username
   password: string # Optional password
   queue: string    # Queue name

 nats: # NATS configuration
   host: string     # NATS host
   port: int        # NATS port
   username: string # Optional username
   password: string # Optional password
   subject: string  # NATS subject
   queue: string    # Queue group name

 memory: # In-memory queue (testing only)
   queue: string # Queue name
 ```

## Usage


 Run the consumer `queue-consumer`


The service will:
1. Read configuration from `config.yaml`
2. Connect to the configured message queue
3. Listen for messages
4. Process each message by:
   - Decoding base64 (if encoded)
   - Parsing JSON content
   - Applying message data to pod template
   - Creating the resulting pod in Kubernetes

## Graceful Shutdown

The service handles SIGINT and SIGTERM signals for graceful shutdown.
