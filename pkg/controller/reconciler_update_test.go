package controller

import (
	"context"
	"testing"

	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	dutyctx "github.com/flanksource/duty/context"
	dutyps "github.com/flanksource/duty/pubsub"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// runningConsumer returns the consumer currently registered for key, or nil.
func runningConsumer(m *ConsumerManager, key types.NamespacedName) *ManagedConsumer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.consumers[key]
}

func newJobTrigger(name, image string) *v1.BatchTrigger {
	trigger := &v1.BatchTrigger{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: v1.Config{
			Job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: name + "-{{.id}}", Namespace: "default"},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "worker", Image: image}},
						},
					},
				},
			},
		},
	}
	trigger.Spec.Memory = &dutyps.MemoryConfig{QueueName: name}
	return trigger
}

func newPodTrigger(name, image string) *v1.BatchTrigger {
	trigger := &v1.BatchTrigger{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: v1.Config{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: name + "-{{.id}}", Namespace: "default"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "worker", Image: image}},
				},
			},
		},
	}
	trigger.Spec.Memory = &dutyps.MemoryConfig{QueueName: name}
	return trigger
}

func TestReconcileReactsToSpecUpdates(t *testing.T) {
	setup := func(t *testing.T, trigger *v1.BatchTrigger) (client.Client, *BatchTriggerReconciler, ctrl.Request) {
		k8s := fake.NewClientBuilder().
			WithScheme(GetScheme()).
			WithStatusSubresource(&v1.BatchTrigger{}).
			WithObjects(trigger).
			Build()

		mgr := NewConsumerManager(dutyctx.NewContext(context.Background()))
		t.Cleanup(mgr.StopAll)

		r := &BatchTriggerReconciler{Client: k8s, Scheme: GetScheme(), Manager: mgr}
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: trigger.Name, Namespace: trigger.Namespace}}
		return k8s, r, req
	}

	t.Run("job image change restarts consumer with new image", func(t *testing.T) {
		g := NewWithT(t)
		k8s, r, req := setup(t, newJobTrigger("job-image", "worker:v1"))

		_, err := r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())
		before := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(before).ToNot(BeNil())
		g.Expect(before.config.Job.Spec.Template.Spec.Containers[0].Image).To(Equal("worker:v1"))

		var trigger v1.BatchTrigger
		g.Expect(k8s.Get(context.Background(), req.NamespacedName, &trigger)).To(Succeed())
		trigger.Spec.Job.Spec.Template.Spec.Containers[0].Image = "worker:v2"
		g.Expect(k8s.Update(context.Background(), &trigger)).To(Succeed())

		_, err = r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())

		after := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(after).ToNot(BeNil())
		g.Expect(after).ToNot(BeIdenticalTo(before), "consumer should be restarted after a spec change")
		g.Expect(after.config.Job.Spec.Template.Spec.Containers[0].Image).To(Equal("worker:v2"))
	})

	t.Run("pod image change restarts consumer with new image", func(t *testing.T) {
		g := NewWithT(t)
		k8s, r, req := setup(t, newPodTrigger("pod-image", "worker:v1"))

		_, err := r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())
		before := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(before).ToNot(BeNil())

		var trigger v1.BatchTrigger
		g.Expect(k8s.Get(context.Background(), req.NamespacedName, &trigger)).To(Succeed())
		trigger.Spec.Pod.Spec.Containers[0].Image = "worker:v2"
		g.Expect(k8s.Update(context.Background(), &trigger)).To(Succeed())

		_, err = r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())

		after := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(after).ToNot(BeNil())
		g.Expect(after).ToNot(BeIdenticalTo(before), "consumer should be restarted after a spec change")
		g.Expect(after.config.Pod.Spec.Containers[0].Image).To(Equal("worker:v2"))
	})

	t.Run("job name change restarts consumer with new name", func(t *testing.T) {
		g := NewWithT(t)
		k8s, r, req := setup(t, newJobTrigger("job-name", "worker:v1"))

		_, err := r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())
		before := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(before).ToNot(BeNil())

		var trigger v1.BatchTrigger
		g.Expect(k8s.Get(context.Background(), req.NamespacedName, &trigger)).To(Succeed())
		trigger.Spec.Job.Name = "job-name-renamed-{{.id}}"
		g.Expect(k8s.Update(context.Background(), &trigger)).To(Succeed())

		_, err = r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())

		after := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(after).ToNot(BeNil())
		g.Expect(after).ToNot(BeIdenticalTo(before), "consumer should be restarted after a spec change")
		g.Expect(after.config.Job.Name).To(Equal("job-name-renamed-{{.id}}"))
	})

	t.Run("queue change restarts consumer with new queue", func(t *testing.T) {
		g := NewWithT(t)
		k8s, r, req := setup(t, newJobTrigger("queue-name", "worker:v1"))

		_, err := r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())
		before := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(before).ToNot(BeNil())

		var trigger v1.BatchTrigger
		g.Expect(k8s.Get(context.Background(), req.NamespacedName, &trigger)).To(Succeed())
		trigger.Spec.Memory.QueueName = "queue-name-v2"
		g.Expect(k8s.Update(context.Background(), &trigger)).To(Succeed())

		_, err = r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())

		after := runningConsumer(r.Manager, req.NamespacedName)
		g.Expect(after).ToNot(BeNil())
		g.Expect(after).ToNot(BeIdenticalTo(before), "consumer should be restarted after a spec change")
		g.Expect(after.config.Memory.QueueName).To(Equal("queue-name-v2"))
	})

	// The reconciler requeues every 30s, so an unchanged spec must not restart
	// the consumer or the queue subscription would be torn down on every requeue.
	t.Run("unchanged spec does not restart consumer", func(t *testing.T) {
		g := NewWithT(t)
		_, r, req := setup(t, newJobTrigger("unchanged", "worker:v1"))

		_, err := r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())
		before := runningConsumer(r.Manager, req.NamespacedName)

		_, err = r.Reconcile(context.Background(), req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(runningConsumer(r.Manager, req.NamespacedName)).To(BeIdenticalTo(before))
	})
}
