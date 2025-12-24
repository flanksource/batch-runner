package controller

import (
	"context"
	"time"

	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups=batch.flanksource.com,resources=batchtriggers,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch.flanksource.com,resources=batchtriggers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

const (
	ConditionTypeReady       = "Ready"
	ConditionTypeProgressing = "Progressing"
	ConditionTypeDegraded    = "Degraded"
)

type BatchTriggerReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Manager *ConsumerManager
}

func (r *BatchTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var trigger v1.BatchTrigger
	if err := r.Get(ctx, req.NamespacedName, &trigger); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("BatchTrigger deleted, stopping consumer")
			r.Manager.Stop(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !trigger.DeletionTimestamp.IsZero() {
		logger.Info("BatchTrigger being deleted, stopping consumer")
		r.Manager.Stop(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	if r.Manager.IsRunning(req.NamespacedName) {
		if err := r.Manager.UpdateConfig(req.NamespacedName, &trigger.Spec); err != nil {
			logger.Error(err, "Failed to update consumer config")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
	} else {
		logger.Info("Starting consumer", "queue", trigger.Spec.String())
		if err := r.Manager.Start(req.NamespacedName, &trigger.Spec); err != nil {
			logger.Error(err, "Failed to start consumer")
			r.setCondition(&trigger, ConditionTypeDegraded, metav1.ConditionTrue, "StartFailed", err.Error())
			if err := r.Status().Update(ctx, &trigger); err != nil {
				logger.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
	}

	stats := r.Manager.GetStats(req.NamespacedName)
	r.updateStatus(&trigger, stats)

	if err := r.Status().Update(ctx, &trigger); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *BatchTriggerReconciler) updateStatus(trigger *v1.BatchTrigger, stats *ConsumerStats) {
	trigger.Status.ConnectionState = stats.ConnectionState
	trigger.Status.MessagesProcessed = stats.MessagesProcessed
	trigger.Status.MessagesFailed = stats.MessagesFailed
	trigger.Status.MessagesRetried = stats.MessagesRetried

	if stats.LastError != "" {
		trigger.Status.LastError = stats.LastError
		t := metav1.NewTime(stats.LastErrorTime)
		trigger.Status.LastErrorTime = &t
	}

	switch stats.ConnectionState {
	case ConnectionStateConnected:
		r.setCondition(trigger, ConditionTypeReady, metav1.ConditionTrue, "Connected", "Consumer is connected and processing messages")
		r.setCondition(trigger, ConditionTypeProgressing, metav1.ConditionFalse, "Stable", "Consumer is stable")
		r.setCondition(trigger, ConditionTypeDegraded, metav1.ConditionFalse, "Healthy", "No errors")
	case ConnectionStateStarting:
		r.setCondition(trigger, ConditionTypeReady, metav1.ConditionFalse, "Starting", "Consumer is starting")
		r.setCondition(trigger, ConditionTypeProgressing, metav1.ConditionTrue, "Starting", "Consumer is initializing")
		r.setCondition(trigger, ConditionTypeDegraded, metav1.ConditionFalse, "Healthy", "No errors")
	case ConnectionStateError:
		r.setCondition(trigger, ConditionTypeReady, metav1.ConditionFalse, "Error", stats.LastError)
		r.setCondition(trigger, ConditionTypeProgressing, metav1.ConditionFalse, "Stopped", "Consumer stopped due to error")
		r.setCondition(trigger, ConditionTypeDegraded, metav1.ConditionTrue, "Error", stats.LastError)
	case ConnectionStateDisconnected:
		r.setCondition(trigger, ConditionTypeReady, metav1.ConditionFalse, "Disconnected", "Consumer is not running")
		r.setCondition(trigger, ConditionTypeProgressing, metav1.ConditionFalse, "Stopped", "Consumer is stopped")
		r.setCondition(trigger, ConditionTypeDegraded, metav1.ConditionFalse, "Stopped", "Consumer is not running")
	}
}

func (r *BatchTriggerReconciler) setCondition(trigger *v1.BatchTrigger, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&trigger.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: trigger.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

func (r *BatchTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.BatchTrigger{}).
		Complete(r)
}
