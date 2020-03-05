package controllers

import (
	"context"
	"crypto/sha256"
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"

	appsV1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	garV1 "github-actions-runner-controller/api/v1alpha1"

	"github.com/go-logr/logr"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ownerKey      = ".metadata.controller"
	kanikoImage   = "gcr.io/kaniko-project/executor:v0.17.1"
	exporterImage = "docker.pkg.github.com/kaidotdev/github-actions-exporter/github-actions-exporter:v0.1.0"
	runnerVersion = "0.2.1"
)

type RunnerReconciler struct {
	client.Client
	Log                 logr.Logger
	Scheme              *runtime.Scheme
	Recorder            record.EventRecorder
	PushRegistryHost    string
	PullRegistryHost    string
	EnableRunnerMetrics bool
}

func (r *RunnerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	runner := &garV1.Runner{}
	ctx := context.Background()
	logger := r.Log.WithValues("runner", req.NamespacedName)
	if err := r.Get(ctx, req.NamespacedName, runner); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.cleanupOwnedResources(ctx, runner); err != nil {
		return ctrl.Result{}, err
	}

	workspaceConfigMap := r.buildWorkspaceConfigMap(runner)

	var foundWorkspaceConfigMap v1.ConfigMap
	if err := r.Client.Get(
		ctx,
		client.ObjectKey{
			Name:      req.Name + "-workspace",
			Namespace: req.Namespace,
		},
		&foundWorkspaceConfigMap,
	); errors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(runner, workspaceConfigMap, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, workspaceConfigMap); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(runner, coreV1.EventTypeNormal, "SuccessfulCreated", "Created workspace config map: %q", workspaceConfigMap.Name)
		logger.V(1).Info("create", "config map", workspaceConfigMap)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	dockerConfigMap := r.buildDockerConfigMap(runner)

	var foundDockerConfigMap v1.ConfigMap
	if err := r.Client.Get(
		ctx,
		client.ObjectKey{
			Name:      req.Name + "-docker",
			Namespace: req.Namespace,
		},
		&foundDockerConfigMap,
	); errors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(runner, dockerConfigMap, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, dockerConfigMap); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(runner, coreV1.EventTypeNormal, "SuccessfulCreated", "Created docker config map: %q", dockerConfigMap.Name)
		logger.V(1).Info("create", "config map", dockerConfigMap)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	deployment := r.buildDeployment(runner)

	var foundDeployment appsV1.Deployment
	if err := r.Client.Get(
		ctx,
		client.ObjectKey{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		&foundDeployment,
	); errors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(runner, deployment, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, deployment); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(runner, coreV1.EventTypeNormal, "SuccessfulCreated", "Created deployment: %q", deployment.Name)
		logger.V(1).Info("create", "deployment", deployment)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RunnerReconciler) buildRepositoryName(runner *garV1.Runner) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(runner.Spec.Image)))[:7]
}

func (r *RunnerReconciler) buildDeployment(runner *garV1.Runner) *appsV1.Deployment {
	env := []coreV1.EnvVar{
		{
			Name:  "REPOSITORY",
			Value: runner.Spec.Repository,
		},
		{
			Name: "TOKEN",
			ValueFrom: &coreV1.EnvVarSource{
				SecretKeyRef: runner.Spec.TokenSecretKeyRef,
			},
		},
		{
			Name: "HOSTNAME",
			ValueFrom: &coreV1.EnvVarSource{
				FieldRef: &coreV1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
	}
	env = append(env, runner.Spec.Template.Spec.Env...)

	containers := []v1.Container{
		{
			Name:            "runner",
			Image:           fmt.Sprintf("%s/%s", r.PullRegistryHost, r.buildRepositoryName(runner)),
			ImagePullPolicy: v1.PullAlways,
			Command: []string{
				"./runner",
			},
			Args: []string{
				"--without-install",
				"--repository=$(REPOSITORY)",
				"--token=$(TOKEN)",
				"--hostname=$(HOSTNAME)",
			},
			Env:       env,
			Resources: runner.Spec.Template.Spec.Resources,
		},
	}
	if r.EnableRunnerMetrics {
		containers = append(containers, v1.Container{
			Name:            "exporter",
			Image:           exporterImage,
			ImagePullPolicy: v1.PullAlways,
			Args: []string{
				"server",
				"--api-address=0.0.0.0:8000",
				"--monitor-address=0.0.0.0:9090",
				"--enable-tracing",
				"--repository=$(REPOSITORY)",
				"--token=$(TOKEN)",
			},
			Env: env,
			Ports: []coreV1.ContainerPort{
				{
					ContainerPort: 9090,
				},
			},
		})
	}

	labels := map[string]string{
		"app": runner.Name,
	}
	for k, v := range runner.Spec.Template.ObjectMeta.Labels {
		labels[k] = v
	}
	runner.Spec.Template.ObjectMeta.Labels = labels
	return &appsV1.Deployment{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      runner.Name,
			Namespace: runner.Namespace,
		},
		Spec: appsV1.DeploymentSpec{
			Selector: &metaV1.LabelSelector{
				MatchLabels: map[string]string{
					"app": runner.Name,
				},
			},
			Replicas: func(i int32) *int32 {
				return &i
			}(1),
			Strategy: appsV1.DeploymentStrategy{
				Type: appsV1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsV1.RollingUpdateDeployment{
					MaxSurge: &intstr.IntOrString{
						Type:   intstr.String,
						StrVal: "25%",
					},
					MaxUnavailable: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 1,
					},
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: runner.Spec.Template.ObjectMeta,
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: v1.PodAffinityTerm{
										LabelSelector: &metaV1.LabelSelector{
											MatchLabels: map[string]string{
												"app": runner.Name,
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:  "kaniko",
							Image: kanikoImage,
							Args: []string{
								"--dockerfile=Dockerfile",
								"--context=dir:///workspace",
								"--cache=true",
								fmt.Sprintf("--destination=%s/%s", r.PushRegistryHost, r.buildRepositoryName(runner)),
							},
							ImagePullPolicy: v1.PullIfNotPresent,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "workspace",
									MountPath: "/workspace",
								},
								{
									Name:      "docker",
									MountPath: "/kaniko/.docker",
								},
							},
							Resources: runner.Spec.BuilderResources,
						},
					},
					Containers: containers,
					Volumes: []v1.Volume{
						{
							Name: "workspace",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: runner.Name + "-workspace",
									},
								},
							},
						},
						{
							Name: "docker",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: runner.Name + "-docker",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *RunnerReconciler) buildWorkspaceConfigMap(runner *garV1.Runner) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      runner.Name + "-workspace",
			Namespace: runner.Namespace,
		},
		Data: map[string]string{
			"Dockerfile": fmt.Sprintf(`
FROM %s
USER root
RUN (command -v apt && apt update && apt install -y ca-certificates) || \
      (command -v apt-get && apt-get update && apt-get install -y ca-certificates) || \
      (command -v dnf && dnf install -y ca-certificates) || \
      (command -v yum && yum install -y ca-certificates) || \
      (command -v zypper && zypper install -n ca-certificates) || \
      (echo "Unknown OS version" && exit 1)
RUN mkdir -p /opt/runner
WORKDIR /opt/runner
ADD https://github.com/kaidotdev/github-actions-runner-controller/releases/download/v%s/runner_%s_linux_amd64 runner 
RUN chmod +x runner
RUN ./runner --only-install
RUN useradd runner
RUN chown -R runner:runner /opt/runner
USER runner
CMD ["./runner"]
`, runner.Spec.Image, runnerVersion, runnerVersion),
		},
	}
}

func (r *RunnerReconciler) buildDockerConfigMap(runner *garV1.Runner) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      runner.Name + "-docker",
			Namespace: runner.Namespace,
		},
		Data: map[string]string{
			"config.json": `{"credsStore":"ecr-login"}`,
		},
	}
}

func (r *RunnerReconciler) cleanupOwnedResources(ctx context.Context, runner *garV1.Runner) error {
	var configMaps v1.ConfigMapList
	if err := r.List(
		ctx,
		&configMaps,
		client.InNamespace(runner.Namespace),
		client.MatchingFields{ownerKey: runner.Name},
	); err != nil {
		return err
	}

	for _, configMap := range configMaps.Items {
		configMap := configMap

		if configMap.Name == runner.Name+"-workspace" || configMap.Name == runner.Name+"-docker" {
			continue
		}

		if err := r.Client.Delete(ctx, &configMap); err != nil {
			return err
		}
		r.Recorder.Eventf(runner, coreV1.EventTypeNormal, "SuccessfulDeleted", "Deleted config map: %q", configMap.Name)
	}

	var deployments appsV1.DeploymentList
	if err := r.List(
		ctx,
		&deployments,
		client.InNamespace(runner.Namespace),
		client.MatchingFields{ownerKey: runner.Name},
	); err != nil {
		return err
	}

	for _, deployment := range deployments.Items {
		deployment := deployment

		if deployment.Name == runner.Name {
			continue
		}

		if err := r.Client.Delete(ctx, &deployment); err != nil {
			return err
		}
		r.Recorder.Eventf(runner, coreV1.EventTypeNormal, "SuccessfulDeleted", "Deleted deployment: %q", deployment.Name)
	}

	return nil
}

func (r *RunnerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(&v1.ConfigMap{}, ownerKey, func(rawObj runtime.Object) []string {
		configMap := rawObj.(*v1.ConfigMap)
		owner := metaV1.GetControllerOf(configMap)
		if owner == nil {
			return nil
		}
		if owner.Kind != "Runner" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(&appsV1.Deployment{}, ownerKey, func(rawObj runtime.Object) []string {
		deployment := rawObj.(*appsV1.Deployment)
		owner := metaV1.GetControllerOf(deployment)
		if owner == nil {
			return nil
		}
		if owner.Kind != "Runner" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&garV1.Runner{}).
		Owns(&v1.ConfigMap{}).
		Owns(&appsV1.Deployment{}).
		Complete(r)
}
