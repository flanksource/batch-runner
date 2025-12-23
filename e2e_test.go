package main

import (
	//"github.com/flanksource/clicky"
	//"github.com/flanksource/commons-test/helm"

	"fmt"
	"strings"

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
			p := clicky.Exec("aws", strings.Split(awsCmd, " ")...).Run()
			logger.Infof(p.Result().Stdout)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))
		})

		It("Should send message and create file", func() {
			fileName := fmt.Sprintf("file-%s", lo.RandomString(10, lo.LettersCharset))
			args := []string{
				fmt.Sprintf(`--endpoint-url=http://localhost:%d`, localStackPort),
				"sqs", "send-message",
				fmt.Sprintf("--queue-url=http://localhost:%d/000000000000/test-batch-runner", localStackPort),
				`--message-body`, fmt.Sprintf(`{"file": "%s"}`, fileName),
				`--region`, `us-east-1`,
			}

			p := clicky.Exec("aws", args...).Run()
			logger.Infof(p.Result().Stdout)
			logger.Infof(p.Result().Stderr)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))

			k := clicky.Exec("kubectl", "exec", controllerPodName, "--", "ls", fmt.Sprintf("/tmp/%s.txt", fileName)).Run()
			logger.Infof(k.Result().Stdout)
			logger.Infof(k.Result().Stderr)
			Expect(k.Err).NotTo(HaveOccurred())
			Expect(k.ExitCode()).To(Equal(0))
		})
	})
})
