package main

import (
	"fmt"
	"time"

	"github.com/flanksource/clicky"
	"github.com/flanksource/commons/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Batch Runner Helm Chart", Ordered, func() {

	var controllerPodName string
	Context("Batch Runner", func() {
		It("Chart is installed", func() {
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

		It("Should create the queues in LocalStack", func() {
			queues := []string{"test-batch-runner-exec", "test-batch-runner-pod", "test-batch-runner-job", "test-batch-runner-pod-spec"}
			for _, queueName := range queues {
				args := []string{
					fmt.Sprintf("--endpoint-url=http://localhost:%d", localStackPort),
					"sqs", "create-queue", "--queue-name", queueName,
					"--region", "us-east-1",
				}
				p := clicky.Exec("aws", args...).WithEnv(awsLocalStackEnv).Run()
				logger.Infof(p.Result().Stdout)
				logger.Infof(p.Result().Stderr)
				Expect(p.Err).NotTo(HaveOccurred())
				Expect(p.ExitCode()).To(Equal(0))
			}
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

			jobName := fmt.Sprintf("job-%s", lo.RandomString(10, lo.LowerCaseLettersCharset))
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

		It("Should apply all pod spec fields to the generated pod", func() {
			result, err := k8s.ApplyFile(ctx, "./fixtures/pod-spec.yaml")
			Expect(err).NotTo(HaveOccurred())
			logger.Infof(result.Pretty().ANSI())

			podLabel := fmt.Sprintf("pod-%s", lo.RandomString(10, lo.LettersCharset))
			annotationValue := fmt.Sprintf("annotation-%s", lo.RandomString(10, lo.LettersCharset))
			envValue := fmt.Sprintf("env-%s", lo.RandomString(10, lo.LettersCharset))

			args := []string{
				fmt.Sprintf(`--endpoint-url=http://localhost:%d`, localStackPort),
				"sqs", "send-message",
				fmt.Sprintf("--queue-url=http://localhost:%d/000000000000/test-batch-runner-pod-spec", localStackPort),
				`--message-body`, fmt.Sprintf(`{"pod_label": "%s", "annotation_value": "%s", "env_value": "%s"}`, podLabel, annotationValue, envValue),
				`--region`, `us-east-1`,
			}

			p := clicky.Exec("aws", args...).WithEnv(awsLocalStackEnv).Run()
			logger.Infof(p.Result().Stdout)
			logger.Infof(p.Result().Stderr)
			Expect(p.Err).NotTo(HaveOccurred())
			Expect(p.ExitCode()).To(Equal(0))

			time.Sleep(10 * time.Second)

			k8s.WaitForPod(ctx, "default", "test-pod-spec", time.Minute*5)

			pod, err := k8s.CoreV1().Pods("default").Get(ctx, "test-pod-spec", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to get test-pod-spec: %v", err)
			Expect(pod).NotTo(BeNil())

			By("Verifying metadata labels")
			Expect(pod.Labels["app"]).To(Equal(podLabel))
			Expect(pod.Labels["batch-runner"]).To(Equal("true"))

			By("Verifying metadata annotations")
			Expect(pod.Annotations["example.com/static"]).To(Equal("test-annotation"))
			Expect(pod.Annotations["example.com/dynamic"]).To(Equal(annotationValue))

			By("Verifying tolerations")
			Expect(pod.Spec.Tolerations).To(ContainElement(corev1.Toleration{
				Key:      "dedicated",
				Operator: corev1.TolerationOpEqual,
				Value:    "batch",
				Effect:   corev1.TaintEffectNoSchedule,
			}))

			By("Verifying nodeSelector")
			Expect(pod.Spec.NodeSelector).To(HaveKeyWithValue("kubernetes.io/os", "linux"))

			By("Verifying securityContext")
			Expect(pod.Spec.SecurityContext).NotTo(BeNil())
			Expect(*pod.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*pod.Spec.SecurityContext.RunAsGroup).To(Equal(int64(3000)))
			Expect(*pod.Spec.SecurityContext.FSGroup).To(Equal(int64(2000)))

			By("Verifying serviceAccountName")
			Expect(pod.Spec.ServiceAccountName).To(Equal("default"))

			By("Verifying initContainers")
			Expect(pod.Spec.InitContainers).To(HaveLen(1))
			Expect(pod.Spec.InitContainers[0].Name).To(Equal("init-setup"))
			Expect(pod.Spec.InitContainers[0].Image).To(Equal("busybox:latest"))

			By("Verifying container environment variables")
			Expect(pod.Spec.Containers).To(HaveLen(1))
			container := pod.Spec.Containers[0]
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "STATIC_ENV", Value: "static-value"}))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "DYNAMIC_ENV", Value: envValue}))

			By("Verifying container resource limits and requests")
			Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("64Mi")))
			Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("50m")))
			Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("32Mi")))
		})
	})
})
