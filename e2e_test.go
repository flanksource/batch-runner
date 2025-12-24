package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Helm tests using fluent interface from commons
var _ = Describe("Batch Runner Helm Chart", Ordered, func() {

	var controllerPodName string
	Context("Batch Runner", func() {
		It("Is Installed", func() {
			status, err := chart.GetStatus()
			if status != nil {
				By(status.Pretty().ANSI())
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Info.Status).To(Equal("deployed"))

			pods, err := k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=batchrunner"})
			Expect(err).ToNot(HaveOccurred())
			Expect(pods.Items).ToNot(BeEmpty())
			pod := lo.FirstOrEmpty(pods.Items)
			controllerPodName = pod.Name
			Expect(string(pod.Status.Phase)).To(Equal("Running"))
		})

		It("LocalStack is running", func() {
			pods, err := k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=localstack"})
			Expect(err).ToNot(HaveOccurred())
			Expect(pods.Items).ToNot(BeEmpty())
			pod := lo.FirstOrEmpty(pods.Items)
			Expect(string(pod.Status.Phase)).To(Equal("Running"))
		})

		It("Should create queue in LocalStack", func() {
			awsCmd := fmt.Sprintf("--endpoint-url=http://localhost:%d sqs create-queue --queue-name test-batch-runner --region us-east-1", localStackPort)
			p := clicky.Exec("aws", strings.Split(awsCmd, " ")...).WithEnv(awsLocalStackEnv).Run()
			logger.Infof(p.Result().Stdout)
			logger.Infof(p.Result().Stderr)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))
		})

		It("Should process message and create file", func() {
			result, err := k8s.ApplyFile(ctx, "./fixtures/exec.yaml")
			Expect(err).NotTo(HaveOccurred())
			logger.Infof(result.Pretty().ANSI())

			fileName := fmt.Sprintf("file-%s", lo.RandomString(10, lo.LettersCharset))
			args := []string{
				fmt.Sprintf(`--endpoint-url=http://localhost:%d`, localStackPort),
				"sqs", "send-message",
				fmt.Sprintf("--queue-url=http://localhost:%d/000000000000/test-batch-runner-exec", localStackPort),
				`--message-body`, fmt.Sprintf(`{"file": "%s"}`, fileName),
				`--region`, `us-east-1`,
			}

			p := clicky.Exec("aws", args...).WithEnv(awsLocalStackEnv).Run()
			logger.Infof(p.Result().Stdout)
			logger.Infof(p.Result().Stderr)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))

			// Wait for sometime before checking if fixture created file
			time.Sleep(10 * time.Second)

			k := clicky.Exec("kubectl", "exec", controllerPodName, "--", "ls", fmt.Sprintf("/tmp/%s.txt", fileName)).Run()
			logger.Infof(k.Result().Stdout)
			logger.Infof(k.Result().Stderr)
			Expect(k.Err).NotTo(HaveOccurred())
			Expect(k.ExitCode()).To(Equal(0))
		})

		It("Should process message and create a pod", func() {
			result, err := k8s.ApplyFile(ctx, "./fixtures/pod.yaml")
			Expect(err).NotTo(HaveOccurred())
			logger.Infof(result.Pretty().ANSI())

			podLabel := fmt.Sprintf("pod-%s", lo.RandomString(10, lo.LettersCharset))
			args := []string{
				fmt.Sprintf(`--endpoint-url=http://localhost:%d`, localStackPort),
				"sqs", "send-message",
				fmt.Sprintf("--queue-url=http://localhost:%d/000000000000/test-batch-runner-pod", localStackPort),
				`--message-body`, fmt.Sprintf(`{"pod_label": "%s"}`, podLabel),
				`--region`, `us-east-1`,
			}

			p := clicky.Exec("aws", args...).WithEnv(awsLocalStackEnv).Run()
			logger.Infof(p.Result().Stdout)
			logger.Infof(p.Result().Stderr)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))

			// Wait for sometime before checking if fixture created file
			time.Sleep(10 * time.Second)

			k8s.WaitForPod(ctx, "default", "test-pod", time.Minute*5)
			Expect(err).NotTo(HaveOccurred())

			pod, err := k8s.CoreV1().Pods("default").Get(ctx, "test-pod", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to get test-pod: %v", err)
			Expect(pod).NotTo(BeNil(), "Expected test-pod to be found")
			Expect(pod.Labels["app"]).To(Equal(podLabel))
		})

		It("Should process message and create a job", func() {
			result, err := k8s.ApplyFile(ctx, "./fixtures/job.yaml")
			Expect(err).NotTo(HaveOccurred())
			logger.Infof(result.Pretty().ANSI())

			jobName := fmt.Sprintf("job-%s", lo.RandomString(10, lo.LettersCharset))
			args := []string{
				fmt.Sprintf(`--endpoint-url=http://localhost:%d`, localStackPort),
				"sqs", "send-message",
				fmt.Sprintf("--queue-url=http://localhost:%d/000000000000/test-batch-runner-job", localStackPort),
				`--message-body`, fmt.Sprintf(`{"job_name": "%s"}`, jobName),
				`--region`, `us-east-1`,
			}

			p := clicky.Exec("aws", args...).WithEnv(awsLocalStackEnv).Run()
			logger.Infof(p.Result().Stdout)
			logger.Infof(p.Result().Stderr)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))

			err = k8s.WaitForJob(ctx, "default", jobName, 5*time.Minute)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
