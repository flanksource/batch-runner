package controller

import (
	v1 "github.com/flanksource/batch-runner/pkg/apis/batch/v1"
	dutyctx "github.com/flanksource/duty/context"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))
}

func GetScheme() *runtime.Scheme {
	return scheme
}

func SetupWithManager(mgr ctrl.Manager, rootCtx dutyctx.Context) error {
	consumerMgr := NewConsumerManager(rootCtx)

	reconciler := &BatchTriggerReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Manager: consumerMgr,
	}

	return reconciler.SetupWithManager(mgr)
}
