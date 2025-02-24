package pkg

import (
	"errors"
	"net/http"
	"os"

	"github.com/flanksource/commons/files"
	"github.com/flanksource/commons/logger"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/net"
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

func IsRetryableError(err error) bool {
	if kerrors.IsBadRequest(err) ||
		kerrors.IsNotAcceptable(err) ||
		kerrors.IsForbidden(err) ||
		kerrors.IsUnauthorized(err) ||
		kerrors.IsRequestEntityTooLargeError(err) {
		return false
	}

	if errors.Is(err, http.ErrHandlerTimeout) ||
		errors.Is(err, http.ErrServerClosed) ||
		net.IsConnectionRefused(err) ||
		net.IsConnectionReset(err) ||
		net.IsProbableEOF(err) ||
		net.IsTimeout(err) {
		return true
	}
	return false
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
