package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/flanksource/clicky"
	flanksourceCtx "github.com/flanksource/commons-db/context"
	"github.com/flanksource/commons-db/kubernetes"
	"github.com/flanksource/commons-test/command"
	"github.com/flanksource/commons-test/helm"
	"github.com/flanksource/commons-test/kind"
	k8stest "github.com/flanksource/commons-test/kubernetes"
	commonsLogger "github.com/flanksource/commons/logger"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/samber/lo"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	kubeconfig       string
	namespace        string
	chartPath        string
	releaseName      string
	ctx              flanksourceCtx.Context
	stopChan         chan struct{}
	localStackPort   int
	awsLocalStackEnv = map[string]string{
		"AWS_ACCESS_KEY_ID":     "test",
		"AWS_SECRET_ACCESS_KEY": "test",
		"AWS_DEFAULT_REGION":    "us-east-1",
	}
)

var logg commonsLogger.Logger
var k8s *kubernetes.Client
var connectionString string

func TestHelm(t *testing.T) {
	logg = commonsLogger.NewWithWriter(GinkgoWriter)
	commonsLogger.Use(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Batch Runner E2E Suite")
}

var chart *helm.HelmChart

var _ = BeforeSuite(func() {

	imageName := "batch-runner"
	imageVersion := "test"
	image := fmt.Sprintf("%s:%s", imageName, imageVersion)

	cluster := kind.NewKind("local").WithServices(kind.ServiceLocalStack)

	By("Docker Build")

	// Build Image and setup kind parallely
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		p := command.NewCommandRunner(true).RunCommand("docker", "build", "-t", image, ".")
		clicky.MustFormat(p.Stdout)
		clicky.MustFormat(p.Stderr)
		Expect(p.ExitCode).To(Equal(0))
		Expect(p.Err).NotTo(HaveOccurred())
		wg.Done()
	}()
	go func() {
		cluster.GetOrCreate().MustSucceed()
		wg.Done()
	}()
	wg.Wait()

	cluster.LoadImage(image)

	// Get environment variables or use defaults
	kubeconfig = lo.CoalesceOrEmpty(
		os.Getenv("KUBECONFIG"),
		filepath.Join(os.Getenv("HOME"), ".kube", "config"),
	)

	namespace = lo.CoalesceOrEmpty(os.Getenv("TEST_NAMESPACE"), "default")

	releaseName = "controller-test"

	logg.Infof("KUBECONFIG=%s ns=%s, chart=%s", kubeconfig, namespace, chartPath)

	if stat, err := os.Stat(kubeconfig); err != nil || stat.IsDir() {
		path, _ := filepath.Abs(kubeconfig)
		Skip(fmt.Sprintf("KUBECONFIG %s is not valid, skipping helm tests", path))
	}

	ctx = flanksourceCtx.New()

	var err error
	k8s, err = ctx.LocalKubernetes(kubeconfig)
	Expect(err).NotTo(HaveOccurred())

	By("Installing Batch Runner")
	chart = helm.NewHelmChart(ctx, "./chart/")

	Expect(chart.
		Release(releaseName).Namespace(namespace).
		WaitFor(time.Minute * 5).
		ForceConflicts().
		Values(map[string]any{
			"image": map[string]any{
				"repository": imageName,
				"tag":        imageVersion,
			},
		}).
		InstallOrUpgrade()).NotTo(HaveOccurred())

	// Ensure localstack running
	localStackPort, stopChan, err = k8stest.PortForwardPod(ctx.Context.Context, k8s.Interface, kubeconfig, namespace, "app.kubernetes.io/name=localstack", 4566)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if stopChan != nil {
		close(stopChan)
	}
})
