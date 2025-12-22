package controller

import (
	"context"
	"testing"

	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	dutyctx "github.com/flanksource/duty/context"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestConsumerStats(t *testing.T) {
	RegisterTestingT(t)

	t.Run("records statistics correctly", func(t *testing.T) {
		RegisterTestingT(t)

		stats := &ConsumerStats{}

		stats.RecordProcessed()
		stats.RecordProcessed()
		Expect(stats.MessagesProcessed).To(Equal(int64(2)))

		stats.RecordFailed(context.DeadlineExceeded)
		Expect(stats.MessagesFailed).To(Equal(int64(1)))
		Expect(stats.LastError).To(Equal("context deadline exceeded"))

		stats.RecordRetried()
		Expect(stats.MessagesRetried).To(Equal(int64(1)))

		stats.SetConnectionState(ConnectionStateConnected)
		Expect(stats.ConnectionState).To(Equal(ConnectionStateConnected))

		snapshot := stats.Snapshot()
		Expect(snapshot.MessagesProcessed).To(Equal(int64(2)))
		Expect(snapshot.MessagesFailed).To(Equal(int64(1)))
		Expect(snapshot.MessagesRetried).To(Equal(int64(1)))
		Expect(snapshot.ConnectionState).To(Equal(ConnectionStateConnected))
	})
}

func TestConsumerManagerUnit(t *testing.T) {
	RegisterTestingT(t)

	t.Run("IsRunning returns false for non-existent consumer", func(t *testing.T) {
		RegisterTestingT(t)

		rootCtx := dutyctx.NewContext(context.Background())
		mgr := NewConsumerManager(rootCtx)

		key := types.NamespacedName{Name: "nonexistent", Namespace: "default"}
		Expect(mgr.IsRunning(key)).To(BeFalse())
	})

	t.Run("GetStats returns disconnected for non-existent consumer", func(t *testing.T) {
		RegisterTestingT(t)

		rootCtx := dutyctx.NewContext(context.Background())
		mgr := NewConsumerManager(rootCtx)

		key := types.NamespacedName{Name: "nonexistent", Namespace: "default"}
		stats := mgr.GetStats(key)
		Expect(stats).ToNot(BeNil())
		Expect(stats.ConnectionState).To(Equal(ConnectionStateDisconnected))
	})

	t.Run("Stop on non-existent consumer is no-op", func(t *testing.T) {
		RegisterTestingT(t)

		rootCtx := dutyctx.NewContext(context.Background())
		mgr := NewConsumerManager(rootCtx)

		key := types.NamespacedName{Name: "nonexistent", Namespace: "default"}
		mgr.Stop(key)
		Expect(mgr.IsRunning(key)).To(BeFalse())
	})

	t.Run("StopAll on empty manager is no-op", func(t *testing.T) {
		RegisterTestingT(t)

		rootCtx := dutyctx.NewContext(context.Background())
		mgr := NewConsumerManager(rootCtx)

		mgr.StopAll()
	})

	t.Run("configChanged detects differences", func(t *testing.T) {
		RegisterTestingT(t)

		config1 := &v1.Config{
			LogLevel: "debug",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod1",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "busybox",
					}},
				},
			},
		}

		config2 := &v1.Config{
			LogLevel: "info",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod2",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "busybox",
					}},
				},
			},
		}

		Expect(configChanged(config1, config2)).To(BeTrue())
		Expect(configChanged(config1, config1)).To(BeFalse())
	})
}
