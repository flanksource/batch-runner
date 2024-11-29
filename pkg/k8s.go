package pkg

import (
	"os"

	"github.com/flanksource/commons/files"
	"github.com/flanksource/commons/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewClient() (kubernetes.Interface, *rest.Config, error) {
	kubeconfigPaths := []string{os.Getenv("KUBECONFIG"), os.ExpandEnv("$HOME/.kube/config")}

	for _, path := range kubeconfigPaths {
		if files.Exists(path) {
			if configBytes, err := os.ReadFile(path); err != nil {
				return nil, nil, err
			} else {
				logger.Infof("Using kubeconfig %s", path)
				client, config, err := NewClientWithConfig(configBytes)
				return client, config, err
			}
		}
	}

	if config, err := rest.InClusterConfig(); err == nil {
		client, err := kubernetes.NewForConfig(config)
		return client, config, err
	} else {
		return nil, nil, err
	}
}

func NewClientWithConfig(kubeConfig []byte) (kubernetes.Interface, *rest.Config, error) {

	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfig)
	if err != nil {
		return nil, nil, err
	}

	if config, err := clientConfig.ClientConfig(); err != nil {
		return nil, nil, err
	} else {
		client, err := kubernetes.NewForConfig(config)
		return client, config, err
	}
}
