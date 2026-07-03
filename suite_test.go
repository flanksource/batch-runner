package main

import (
	"fmt"
	"os"
	"path/filepath"
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

const (
	localStackWaitTimeout = 10 * time.Minute

	// Since localstack/helm-charts#148 (Mar 2026) the chart defaults to the
	// localstack/localstack-pro image, which requires an auth token and never
	// becomes ready without one. Pin the community image at the version that
	// was current when this suite was introduced.
	localStackImageRepo = "localstack/localstack"
	localStackImageTag  = "4.12.0"
)

// dumpKubeDiagnostics prints pod, event and localstack log details so a
// failed helm --wait surfaces the underlying pod state in CI logs.
func dumpKubeDiagnostics(ns string) {
	runner := command.NewCommandRunner(true)
	for _, args := range [][]string{
		{"get", "pods", "-n", ns, "-o", "wide"},
		{"get", "events", "-n", ns, "--sort-by=.lastTimestamp"},
		{"describe", "deployment", "-n", ns},
		{"logs", "-n", ns, "-l", "app.kubernetes.io/name=localstack", "--tail=100"},
	} {
		p := runner.RunCommand("kubectl", args...)
		clicky.MustFormat(p.Stdout)
		clicky.MustFormat(p.Stderr)
	}
}

var _ = BeforeSuite(func() {

	imageName := "batch-runner"
	imageVersion := "test"
	image := fmt.Sprintf("%s:%s", imageName, imageVersion)
	localStackImage := fmt.Sprintf("%s:%s", localStackImageRepo, localStackImageTag)

	cluster := kind.NewKind("local")

	By("Docker Build")

	p := command.NewCommandRunner(true).RunCommand("docker", "build", "-t", image, ".")
	clicky.MustFormat(p.Stdout)
	clicky.MustFormat(p.Stderr)
	Expect(p.ExitCode).To(Equal(0))
	Expect(p.Err).NotTo(HaveOccurred())

	// Pull on the host and side-load into kind so the node never pulls from
	// Docker Hub (unauthenticated in-node pulls are rate-limited on CI).
	By("Pulling LocalStack image")
	p = command.NewCommandRunner(true).RunCommand("docker", "pull", localStackImage)
	clicky.MustFormat(p.Stdout)
	clicky.MustFormat(p.Stderr)
	Expect(p.ExitCode).To(Equal(0))
	Expect(p.Err).NotTo(HaveOccurred())

	cluster.GetOrCreate().MustSucceed()

	cluster.LoadImage(image)
	cluster.LoadImage(localStackImage)

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

	By("Installing Localstack")
	err = helm.NewHelmChart(ctx, "localstack/localstack").
		Repository("localstack", "https://localstack.github.io/helm-charts").
		Release("localstack").
		Namespace(namespace).
		WaitFor(localStackWaitTimeout).
		Values(map[string]any{
			"image": map[string]any{
				"repository": localStackImageRepo,
				"tag":        localStackImageTag,
				"pullPolicy": "IfNotPresent",
			},
		}).
		InstallOrUpgrade()
	if err != nil {
		dumpKubeDiagnostics(namespace)
	}
	Expect(err).NotTo(HaveOccurred())

	By("Installing Batch Runner")
	chart = helm.NewHelmChart(ctx, "./chart/")

	err = chart.
		Release(releaseName).Namespace(namespace).
		WaitFor(time.Minute * 5).
		ForceConflicts().
		Values(map[string]any{
			"image": map[string]any{
				"repository": imageName,
				"tag":        imageVersion,
			},
		}).
		InstallOrUpgrade()
	if err != nil {
		dumpKubeDiagnostics(namespace)
	}
	Expect(err).NotTo(HaveOccurred())

	// Ensure localstack running
	localStackPort, stopChan, err = k8stest.PortForwardPod(ctx.Context.Context, k8s.Interface, kubeconfig, namespace, "app.kubernetes.io/name=localstack", 4566)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if stopChan != nil {
		close(stopChan)
	}
})
