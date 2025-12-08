package pkg_test

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SNS to SQS Integration", Ordered, func() {

	DescribeTable("should render message templates correctly",
		func(message, expectedPodName, expectedMsgJSON, expectedUIMsg string) {
			_, err := snsClient.Publish(context.TODO(), &sns.PublishInput{
				TopicArn: &topicArn,
				Message:  aws.String(message),
			})
			Expect(err).To(BeNil())

			findPod := func() *corev1.Pod {
				pod, e := client.CoreV1().Pods("default").Get(context.TODO(), expectedPodName, v1.GetOptions{})
				if e == nil {
					return pod
				}
				return nil
			}

			Eventually(findPod).
				WithTimeout(10 * time.Second).
				WithPolling(time.Second).
				ShouldNot(BeNil())

			pod := findPod()
			Expect(pod.Name).To(Equal(expectedPodName))
			Expect(pod.Annotations["msg"]).To(ContainSubstring(expectedMsgJSON))

			var uiMessageArg string
			for _, arg := range pod.Spec.Containers[0].Command {
				if strings.Contains(arg, "--ui-message=") {
					uiMessageArg = strings.SplitN(arg, "=", 2)[1]
					break
				}
			}
			Expect(uiMessageArg).To(ContainSubstring(expectedUIMsg))
		},
		Entry("simple message",
			`{"a": "first"}`,
			"batch-first",
			`"a":"first"`,
			"first",
		),
		Entry("message with extra field",
			`{"a": "second", "extra": "data"}`,
			"batch-second",
			`"a":"second"`,
			"second",
		),
		Entry("message with nested object",
			`{"a": "third", "nested": {"key": "value"}}`,
			"batch-third",
			`"a":"third"`,
			"third",
		),
	)
})
