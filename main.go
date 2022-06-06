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
	var exporterImage string
	var kanikoImage string
	var binaryVersion string
	var runnerVersion string
	var disableupdate bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager.")
	flag.StringVar(&pushRegistryHost, "push-registry-host", "ghcr.io/kaidotdev/github-actions-runner-controller", "Host of Docker Registry used as push destination.")
	flag.StringVar(&pullRegistryHost, "pull-registry-host", "ghcr.io/kaidotdev/github-actions-runner-controller", "Host of Docker Registry used as pull source.")
	flag.BoolVar(&enableRunnerMetrics, "enable-runner-metrics", false, "Enable to expose runner metrics using prometheus exporter.")
	flag.StringVar(&exporterImage, "exporter-image", "ghcr.io/kaidotdev/github-actions-exporter/github-actions-exporter:v0.1.1", "Docker Image of exporter used by exporter container")
	flag.StringVar(&kanikoImage, "kaniko-image", "gcr.io/kaniko-project/executor:v0.18.0", "Docker Image of kaniko used by builder container")
	flag.StringVar(&binaryVersion, "binary-version", "0.3.14", "Version of own runner binary")
	flag.StringVar(&runnerVersion, "runner-version", "2.291.1", "Version of GitHub Actions runner")
	flag.BoolVar(&disableupdate, "disableupdate", false, "Disable self-hosted runner automatic update to the latest released version")
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
		ExporterImage:       exporterImage,
		KanikoImage:         kanikoImage,
		BinaryVersion:       binaryVersion,
		RunnerVersion:       runnerVersion,
		Disableupdate:       disableupdate,
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
