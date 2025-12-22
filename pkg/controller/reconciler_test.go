//go:build integration

package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	dutyctx "github.com/flanksource/duty/context"
	dutyKubernetes "github.com/flanksource/duty/kubernetes"
	dutyps "github.com/flanksource/duty/pubsub"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestBatchTriggerReconciler(t *testing.T) {
	RegisterTestingT(t)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err := testEnv.Start()
	Expect(err).To(BeNil())
	Expect(cfg).ToNot(BeNil())
	defer testEnv.Stop()

	err = v1.AddToScheme(clientgoscheme.Scheme)
	Expect(err).To(BeNil())

	k8sClient, err := client.New(cfg, client.Options{Scheme: clientgoscheme.Scheme})
	Expect(err).To(BeNil())
	Expect(k8sClient).ToNot(BeNil())

	clientset := kubernetes.NewForConfigOrDie(cfg)
	rootCtx := dutyctx.New()
	rootCtx = rootCtx.WithLocalKubernetes(dutyKubernetes.NewKubeClient(rootCtx.Logger, clientset, cfg))

	consumerMgr := NewConsumerManager(rootCtx)

	reconciler := &BatchTriggerReconciler{
		Client:  k8sClient,
		Scheme:  clientgoscheme.Scheme,
		Manager: consumerMgr,
	}

	t.Run("creates and updates BatchTrigger status", func(t *testing.T) {
		RegisterTestingT(t)

		trigger := &v1.BatchTrigger{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trigger",
				Namespace: "default",
			},
			Spec: v1.Config{
				LogLevel: "debug",
				Pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-{{.id}}",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "test",
							Image: "busybox",
						}},
					},
				},
			},
		}
		trigger.Spec.Memory = &dutyps.MemoryConfig{QueueName: "test-queue"}

		err := k8sClient.Create(context.Background(), trigger)
		Expect(err).To(BeNil())

		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-trigger",
				Namespace: "default",
			},
		}

		result, err := reconciler.Reconcile(context.Background(), req)
		Expect(err).To(BeNil())
		Expect(result.RequeueAfter).To(Equal(30 * time.Second))

		Expect(consumerMgr.IsRunning(req.NamespacedName)).To(BeTrue())

		var updated v1.BatchTrigger
		err = k8sClient.Get(context.Background(), req.NamespacedName, &updated)
		Expect(err).To(BeNil())
		Expect(updated.Status.ConnectionState).ToNot(BeEmpty())

		err = k8sClient.Delete(context.Background(), trigger)
		Expect(err).To(BeNil())

		result, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).To(BeNil())

		Expect(consumerMgr.IsRunning(req.NamespacedName)).To(BeFalse())
	})

	t.Run("stops consumer when BatchTrigger is deleted", func(t *testing.T) {
		RegisterTestingT(t)

		trigger := &v1.BatchTrigger{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-trigger-delete",
				Namespace: "default",
			},
			Spec: v1.Config{
				Pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "delete-test-{{.id}}",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "test",
							Image: "busybox",
						}},
					},
				},
			},
		}
		trigger.Spec.Memory = &dutyps.MemoryConfig{QueueName: "delete-test-queue"}

		err := k8sClient.Create(context.Background(), trigger)
		Expect(err).To(BeNil())

		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-trigger-delete",
				Namespace: "default",
			},
		}

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).To(BeNil())
		Expect(consumerMgr.IsRunning(req.NamespacedName)).To(BeTrue())

		err = k8sClient.Delete(context.Background(), trigger)
		Expect(err).To(BeNil())

		_, err = reconciler.Reconcile(context.Background(), req)
		Expect(err).To(BeNil())
		Expect(consumerMgr.IsRunning(req.NamespacedName)).To(BeFalse())
	})
}
