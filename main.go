package main

import (
	"flag"
	"github-actions-runner-controller/controllers"
	"os"

	garV1 "github-actions-runner-controller/api/v1"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = garV1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var pushRegistryHost string
	var pullRegistryHost string
	var enableRunnerMetrics bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager.")
	flag.StringVar(&pushRegistryHost, "push-registry-host", "docker.pkg.github.com/kaidotdev/github-actions-runner-controller", "Host of Docker Registry used as push destination.")
	flag.StringVar(&pullRegistryHost, "pull-registry-host", "docker.pkg.github.com/kaidotdev/github-actions-runner-controller", "Host of Docker Registry used as pull source.")
	flag.BoolVar(&enableRunnerMetrics, "enable-runner-metrics", false, "Enable to expose runner metrics using by prometheus exporter.")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "github-actions-runner-controller",
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controllers.RunnerReconciler{
		Client:              mgr.GetClient(),
		Log:                 ctrl.Log.WithName("controllers").WithName("Runner"),
		Scheme:              mgr.GetScheme(),
		Recorder:            mgr.GetEventRecorderFor("github-actions-runner-controller"),
		PushRegistryHost:    pushRegistryHost,
		PullRegistryHost:    pullRegistryHost,
		EnableRunnerMetrics: enableRunnerMetrics,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Runner")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
