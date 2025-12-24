package cmd

import (
	"os"

	"github.com/flanksource/batch-runner/pkg/controller"
	"github.com/flanksource/commons/logger"
	"github.com/flanksource/duty/context"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	metricsAddr          string
	probeAddr            string
	enableLeaderElection bool
)

var ControllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Run as Kubernetes controller watching BatchTrigger resources",
	Run:   runController,
}

func init() {
	ControllerCmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	ControllerCmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	ControllerCmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
}

func runController(cmd *cobra.Command, args []string) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	dutyCtx := context.New()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: controller.GetScheme(),
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "batch-runner.flanksource.com",
	})
	if err != nil {
		logger.Fatalf("unable to start manager: %v", err)
		os.Exit(1)
	}

	if err := controller.SetupWithManager(mgr, dutyCtx); err != nil {
		logger.Fatalf("unable to create controller: %v", err)
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Fatalf("unable to set up health check: %v", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Fatalf("unable to set up ready check: %v", err)
		os.Exit(1)
	}

	logger.Infof("Starting controller manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Fatalf("problem running manager: %v", err)
		os.Exit(1)
	}
}
