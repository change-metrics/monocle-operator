/*
Copyright 2023 Monocle developers.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"os"
	"strconv"
	"time"

	monoclev1alpha1 "github.com/change-metrics/monocle-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl_util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/thanhpk/randstr"
)

// The order of groups metters. apps -> v1 -> monocle.monocle.change-metrics.io
// +kubebuilder:rbac:groups=monocle.monocle.change-metrics.io,resources=monocles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monocle.monocle.change-metrics.io,resources=monocles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monocle.monocle.change-metrics.io,resources=monocles/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// +kubebuilder:rbac:groups=v1,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=v1,resources=services/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
// +kubebuilder:rbac:groups=v1,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=v1,resources=configmaps/status,verbs=get
// +kubebuilder:rbac:groups=v1,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=v1,resources=secrets/status,verbs=get
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get

// MonocleReconciler reconciles a Monocle object
type MonocleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *MonocleReconciler) rollOutWhenApiSecretsChange(ctx context.Context, logger logr.Logger, depl appsv1.Deployment, apiSecretsVersion string) error {
	previousSecretsVersion := depl.Spec.Template.Annotations["apiSecretsVersion"]
	if previousSecretsVersion != apiSecretsVersion {
		logger.Info("Start a rollout due to secrets update",
			"name", depl.Name,
			"previous secrets version", previousSecretsVersion,
			"new secrets version", apiSecretsVersion)
		depl.Spec.Template.Annotations["apiSecretsVersion"] = apiSecretsVersion
		return r.Update(ctx, &depl)
	}
	return nil
}

func triggerUpdateIdentsJob(
	r *MonocleReconciler, ctx context.Context, instance monoclev1alpha1.Monocle, namespace string,
	logger logr.Logger, elasticUrlEnvVar corev1.EnvVar, apiConfigMapName string, monocleImage string) error {

	jobname := "update-idents-job"
	job := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobname,
			Namespace: namespace,
		},
	}

	// Checking if there is a Job Resource by Name
	err := r.Client.Get(ctx,
		client.ObjectKey{Name: jobname, Namespace: namespace},
		&job)
	// Delete it if there is an old job resource
	fg := metav1.DeletePropagationBackground
	if err == nil {
		r.Client.Delete(ctx,
			&job, &client.DeleteOptions{PropagationPolicy: &fg})
	}

	// Job Spec Container Adaptation
	apiConfigMapVolumeName := "api-cm-volume"
	// Adding the New Container Definition
	ttlSecondsAfterFinished := int32(3600)

	jobToCreate := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobname,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: "Never",
					Containers: []corev1.Container{
						{
							Name:    jobname,
							Image:   monocleImage,
							Command: []string{"bash"},
							Args:    []string{"-c", " monocle janitor update-idents --elastic ${MONOCLE_ELASTIC_URL} --config /etc/monocle/config.yaml"},
							Env: []corev1.EnvVar{
								elasticUrlEnvVar,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      apiConfigMapVolumeName,
									ReadOnly:  true,
									MountPath: "/etc/monocle",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: apiConfigMapVolumeName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: apiConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if err := ctrl_util.SetControllerReference(&instance, &jobToCreate, r.Scheme); err != nil {
		logger.Info("Unable to set controller reference", "name", jobname)
	}

	return r.Create(ctx, &jobToCreate)
}

func serviceStatusConverter(isReady bool) string {
	if isReady {
		return "Ready"
	}
	return "In Progress ..."
}

// IsDeploymentRolloutDone returns True when the Deployment rollout is over
func IsDeploymentRolloutDone(obj *appsv1.Deployment) bool {
	return obj.Status.ObservedGeneration >= obj.Generation &&
		obj.Status.Replicas == obj.Status.ReadyReplicas &&
		obj.Status.Replicas == obj.Status.AvailableReplicas
}

func isDeploymentReady(dep *appsv1.Deployment) bool {
	return dep.Status.ReadyReplicas > 0 && IsDeploymentRolloutDone(dep)
}

func reconcileLater(err error) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: time.Second * 5}, err
}

func stopReconcile() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *MonocleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		logger   = log.FromContext(ctx)
		instance = monoclev1alpha1.Monocle{}
		ns       = req.Namespace
	)

	logger.Info("Enter Reconcile ...")

	// Get the Monocle instance related to request
	err := r.Client.Get(ctx, req.NamespacedName, &instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found. Return and don't requeue.
			logger.Info("Instance object not found. Stop reconcile.")
			// Stop reconcile
			return stopReconcile()
		}
		// Error reading the object - requeue the request.
		logger.Info("Unable to read the Monocle object. Reconcile continues ...")
		// Stop reconcile
		return reconcileLater(err)
	}
	return r.ReconcileStep(ctx, ns, instance, false)
}

func (r *MonocleReconciler) StandaloneReconcile(ctx context.Context, ns string, instance monoclev1alpha1.Monocle) {
	d, _ := time.ParseDuration("5s")
	for {
		result, err := r.ReconcileStep(ctx, ns, instance, true)
		if (result == ctrl.Result{} && err == nil) {
			break
		}
		log.Log.Info("Waiting 5s for the next reconcile call ...")
		time.Sleep(d)
	}
	log.Log.Info("Standalone reconcile done.")
	os.Exit(0)
}

func (r *MonocleReconciler) ReconcileStep(ctx context.Context, ns string, instance monoclev1alpha1.Monocle, standalone bool) (ctrl.Result, error) {
	var (
		runAsNonRoot             = true
		allowPrivilegeEscalation = false
		err                      error
		logger                   = log.FromContext(ctx)
		owner                    client.Object
	)

	logger.Info("Enter ReconcileStep ...")

	// Utility to build a name prepended with the Monocle instance's name
	resourceName := func(rName string) string { return instance.Name + "-" + rName }

	if standalone {
		controllerCMName := resourceName("fake-controller")
		controllerCM := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controllerCMName,
				Namespace: ns,
			}}
		err = r.Client.Get(
			ctx, client.ObjectKey{Name: controllerCMName, Namespace: ns}, &controllerCM)
		if err != nil && k8s_errors.IsNotFound(err) {
			controllerCM.Data = nil
			logger.Info("Creating ConfigMap", "name", controllerCMName)
			// Create the fake controller configMap
			if err := r.Create(ctx, &controllerCM); err != nil {
				logger.Error(err, "Unable to create configMap", "name", controllerCMName)
				return reconcileLater(err)
			}
		} else if err != nil {
			// Handle the unexpected err
			logger.Error(err, "Unable to get resource", "name", controllerCMName)
			return reconcileLater(err)
		} else {
			// Eventually handle resource update
			logger.Info("Resource fetched successfuly", "name", controllerCMName)
		}
		owner = controllerCM.DeepCopy()
	}

	////////////////////////////////////////////////////////
	//  Handle the Monocle Elastic StatefulSet instance   //
	////////////////////////////////////////////////////////

	// Handle service for elastic //
	////////////////////////////////

	elasticPort := 9200
	elasticMatchLabels := map[string]string{
		"app":  "monocle",
		"tier": "elastic",
	}
	elasticServiceName := resourceName("elastic")
	elasticService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      elasticServiceName,
			Namespace: ns,
		},
	}
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: elasticServiceName, Namespace: ns}, &elasticService)
	if err != nil && k8s_errors.IsNotFound(err) {
		elasticService.Spec = corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     resourceName("elastic-port"),
					Protocol: corev1.ProtocolTCP,
					Port:     int32(elasticPort),
				},
			},
			Selector: elasticMatchLabels,
		}
		if err := ctrl_util.SetControllerReference(owner, &elasticService, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", elasticServiceName)
			return reconcileLater(err)
		}
		logger.Info("Creating Service", "name", elasticServiceName)
		if err := r.Create(ctx, &elasticService); err != nil {
			logger.Error(err, "Unable to create service", "name", elasticService)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", elasticServiceName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", elasticServiceName)
	}

	elasticUrlEnvVar := corev1.EnvVar{
		Name:  "MONOCLE_ELASTIC_URL",
		Value: "http://" + elasticServiceName + ":" + strconv.Itoa(elasticPort),
	}

	// Handle the elactic deployment //
	///////////////////////////////////

	elasticStatefulSetName := resourceName("elastic")
	elasticStatefulSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      elasticStatefulSetName,
			Namespace: ns,
		},
	}
	elasticReplicasCount := int32(1)

	// Default to 1Gi
	defaultSize := resource.NewQuantity(1*10^9, resource.DecimalSI)

	var elasticPVCStorageQuantity resource.Quantity
	if instance.Spec.StorageSize != "" {
		elasticPVCStorageQuantity, err = resource.ParseQuantity(instance.Spec.StorageSize)
		if err != nil {
			logger.Error(err, "Unable to parse provided storageSize, use default", instance.Spec.StorageSize)
			elasticPVCStorageQuantity = *defaultSize
		}
	} else {
		elasticPVCStorageQuantity = *defaultSize
	}

	elasticSearchReady := func() bool {
		return elasticReplicasCount == elasticStatefulSet.Status.ReadyReplicas
	}

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: elasticStatefulSetName, Namespace: ns}, &elasticStatefulSet)
	if err != nil && k8s_errors.IsNotFound(err) {
		elasticDataVolumeName := resourceName("elastic-data-volume")
		// Once created StatefulSet selector is immutable
		elasticStatefulSet.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: elasticMatchLabels,
		}
		// Set replicas count
		elasticStatefulSet.Spec.Replicas = &elasticReplicasCount
		// Set the volume claim templates
		volumeClaimSpec := corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					"storage": elasticPVCStorageQuantity,
				},
			},
		}
		if instance.Spec.StorageClassName != "" {
			volumeClaimSpec.StorageClassName = &instance.Spec.StorageClassName
		}
		elasticStatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      elasticDataVolumeName,
					Namespace: ns,
				},
				Spec: volumeClaimSpec,
			},
		}

		// Set the StatefulSet pod template
		elasticStatefulSet.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: elasticMatchLabels,
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
				Containers: []corev1.Container{
					{
						Name:  resourceName("elastic-pod"),
						Image: "docker.elastic.co/elasticsearch/elasticsearch:7.17.5",
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "ES_JAVA_OPTS",
								Value: "-Xms512m -Xmx512m",
							},
							{
								Name:  "discovery.type",
								Value: "single-node",
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      elasticDataVolumeName,
								MountPath: "/usr/share/elasticsearch/data",
							},
						},
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: int32(elasticPort),
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/_cluster/health",
									Port: intstr.FromInt(elasticPort),
								},
							},
							TimeoutSeconds:   30,
							FailureThreshold: 6,
						},
					},
				},
			},
		}
		// Add owner reference
		if err := ctrl_util.SetControllerReference(owner, &elasticStatefulSet, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", elasticStatefulSetName)
			return reconcileLater(err)
		}
		// Create the resource
		logger.Info("Creating StatefulSet", "name", elasticStatefulSetName)
		if err := r.Create(ctx, &elasticStatefulSet); err != nil {
			logger.Error(err, "Unable to create deployment", "name", elasticStatefulSetName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", elasticStatefulSetName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", elasticStatefulSetName)
	}

	////////////////////////////////////////////////////////
	//       Handle the Monocle API Secret Instance       //
	////////////////////////////////////////////////////////

	// This secret contains environment variables required by the
	// API and/or crawlers. The CRAWLERS_API_KEY entry is
	// mandatory for crawlers to authenticate against the API.

	apiSecretName := resourceName("api")
	apiSecretData := map[string][]byte{
		"CRAWLERS_API_KEY": []byte(randstr.String(24))}
	apiSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiSecretName,
			Namespace: ns},
	}
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiSecretName, Namespace: ns}, &apiSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		apiSecret.Data = apiSecretData
		if err := ctrl_util.SetControllerReference(owner, &apiSecret, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", apiSecretName)
			return reconcileLater(err)
		}
		logger.Info("Creating secret", "name", apiSecretName)
		// Create the resource
		if err := r.Create(ctx, &apiSecret); err != nil {
			logger.Error(err, "Unable to create secret", "name", apiSecretName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", apiSecretName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiSecretName)
	}

	apiSecretsVersion := apiSecret.ResourceVersion
	logger.Info("apiSecret resource", "version", apiSecretsVersion)

	////////////////////////////////////////////////////////
	//     Handle the Monocle API ConfigMap Instance      //
	////////////////////////////////////////////////////////

	apiConfigMapName := resourceName("api")
	apiConfigMapData := map[string]string{
		"config.yaml": `
workspaces:
  - name: demo
    crawlers: []
`}
	apiConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiConfigMapName,
			Namespace: ns},
	}

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiConfigMapName, Namespace: ns}, &apiConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		apiConfigMap.Data = apiConfigMapData
		if err := ctrl_util.SetControllerReference(owner, &apiConfigMap, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", apiConfigMapName)
			return reconcileLater(err)
		}
		logger.Info("Creating ConfigMap", "name", apiConfigMapName)
		// Create the secret
		if err := r.Create(ctx, &apiConfigMap); err != nil {
			logger.Error(err, "Unable to create configMap", "name", apiConfigMap)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", apiConfigMapName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiConfigMapName)
	}

	apiConfigVersion := apiConfigMap.ResourceVersion
	logger.Info("apiConfig resource", "version", apiConfigVersion)

	////////////////////////////////////////////////////////
	//     Handle the Monocle API Deployment instance     //
	////////////////////////////////////////////////////////

	// Handle service for api //
	////////////////////////////

	apiPort := 8080
	apiMatchLabels := map[string]string{
		"app":  "monocle",
		"tier": "api",
	}
	apiServiceName := resourceName("api")
	apiService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiServiceName,
			Namespace: ns,
		},
	}
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiServiceName, Namespace: ns}, &apiService)
	if err != nil && k8s_errors.IsNotFound(err) {
		apiService.Spec = corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     resourceName("api-port"),
					Protocol: corev1.ProtocolTCP,
					Port:     int32(apiPort),
				},
			},
			Selector: apiMatchLabels,
		}
		if err := ctrl_util.SetControllerReference(owner, &apiService, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", apiServiceName)
			return reconcileLater(err)
		}
		logger.Info("Creating Service", "name", apiServiceName)
		if err := r.Create(ctx, &apiService); err != nil {
			logger.Error(err, "Unable to create service", "name", apiService)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", apiServiceName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiServiceName)
	}

	// Handle API deployment //
	///////////////////////////

	apiDeploymentName := resourceName("api")

	monoclePublicURL := instance.Spec.PublicURL
	logger.Info("Monocle public URL set to", "url", monoclePublicURL)

	var currentApiDeployment appsv1.Deployment
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiDeploymentName, Namespace: ns}, &currentApiDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		apiDeployment := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      apiDeploymentName,
				Namespace: ns,
			},
		}
		apiReplicasCount := int32(1)
		// Setup the deployment object
		apiConfigMapVolumeName := resourceName("api-cm-volume")
		// Once created Deployment selector is immutable
		apiDeployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: apiMatchLabels,
		}
		// TODO - Set the strategy
		// Set replicas count
		apiDeployment.Spec.Replicas = &apiReplicasCount
		// Set the Deployment annotations
		apiDeployment.Annotations = map[string]string{
			"apiConfigVersion": apiConfigVersion,
		}

		// Set the Deployment pod template
		apiDeployment.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: apiMatchLabels,
				Annotations: map[string]string{
					"apiSecretsVersion": apiSecretsVersion,
					"monocleImage":      instance.Spec.Image,
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
				Containers: []corev1.Container{
					{
						Name: resourceName("api-pod"),
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						Image:   instance.Spec.Image,
						Command: []string{"monocle", "api"},
						EnvFrom: []corev1.EnvFromSource{
							{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: apiSecretName,
									},
								},
							},
						},
						Env: []corev1.EnvVar{
							elasticUrlEnvVar,
							{
								Name:  "MONOCLE_PUBLIC_URL",
								Value: monoclePublicURL,
							},
						},
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: int32(apiPort),
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt(apiPort),
								},
							},
							TimeoutSeconds:   30,
							FailureThreshold: 6,
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      apiConfigMapVolumeName,
								ReadOnly:  true,
								MountPath: "/etc/monocle",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: apiConfigMapVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: apiConfigMapName,
								},
							},
						},
					},
				},
			},
		}
		if err := ctrl_util.SetControllerReference(owner, &apiDeployment, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", apiDeploymentName)
			return reconcileLater(err)
		}
		logger.Info("Creating Deployment", "name", apiDeploymentName)
		// Create the resource
		if err := r.Create(ctx, &apiDeployment); err != nil {
			logger.Error(err, "Unable to create deployment", "name", apiDeploymentName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", apiDeploymentName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiDeploymentName)

		// Handle Monocle image update
		if currentApiDeployment.Spec.Template.Annotations["monocleImage"] != instance.Spec.Image {
			currentApiDeployment.Spec.Template.Annotations["monocleImage"] = instance.Spec.Image
			currentApiDeployment.Spec.Template.Spec.Containers[0].Image = instance.Spec.Image
			// Update Deployment Resource to set the new container image
			logger.Info("Start a rollout of the api due to container image change")
			err := r.Update(ctx, &currentApiDeployment)
			if err != nil {
				logger.Error(err, "Unable to update resource", "name", apiDeploymentName)
			}
			return reconcileLater(err)
		}

		// Check pod template Annotation secretsVersion
		err := r.rollOutWhenApiSecretsChange(ctx, logger, currentApiDeployment, apiSecretsVersion)
		if err != nil {
			logger.Info("Unable to update spec deployment annotations", "name", apiDeploymentName)
			reconcileLater(err)
		}

		// Check if Deployment Pod Annotation for ConfigMap resource version was updated
		previousVersion := currentApiDeployment.Annotations["apiConfigVersion"]
		if previousVersion != apiConfigVersion {

			logger.Info("Start the update-idents jobs because of api configMap update",
				"name", currentApiDeployment.Name,
				"previous configmap version", previousVersion,
				"new configmap version", apiConfigVersion)
			currentApiDeployment.Annotations["apiConfigVersion"] = apiConfigVersion
			// Update Deployment Resource to set the new configMap resource version
			err := r.Update(ctx, &currentApiDeployment)
			if err != nil {
				logger.Error(err, "Unable to update resource", "name", apiDeploymentName)
				return reconcileLater(err)
			}
			// Trigger the job
			err = triggerUpdateIdentsJob(r, ctx, instance, ns, logger, elasticUrlEnvVar, apiConfigMapName, instance.Spec.Image)
			if err != nil {
				logger.Error(err, "Unable to trigger update-idents", "name", err)
				return reconcileLater(err)
			}
		}
	}

	////////////////////////////////////////////////////////
	//   Handle the Monocle Crawler Deployment instance   //
	////////////////////////////////////////////////////////

	monocleAPIInternalURL := "http://" + apiServiceName + ":" + strconv.Itoa(apiPort)

	crawlerDeploymentName := resourceName("crawler")

	var currentCrawlerDeployment appsv1.Deployment
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: crawlerDeploymentName, Namespace: ns}, &currentCrawlerDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		crawlerDeployment := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crawlerDeploymentName,
				Namespace: ns,
			},
		}
		crawlerReplicasCount := int32(1)
		crawlerPort := 9001
		crawlerMatchLabels := map[string]string{
			"app":  "monocle",
			"tier": "crawler",
		}
		// Setup the deployment
		crawlerConfigMapVolumeName := resourceName("crawler-cm-volume")
		// Once created Deployment selector is immutable
		crawlerDeployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: crawlerMatchLabels,
		}
		// TODO - Set the strategy
		// Set replicas count
		crawlerDeployment.Spec.Replicas = &crawlerReplicasCount
		// Set the Deployment pod template
		crawlerDeployment.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: crawlerMatchLabels,
				Annotations: map[string]string{
					"apiSecretsVersion": apiSecretsVersion,
					"monocleImage":      instance.Spec.Image,
				},
			},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyAlways,
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: &runAsNonRoot,
					SeccompProfile: &corev1.SeccompProfile{
						Type: "RuntimeDefault",
					},
				},
				Containers: []corev1.Container{
					{
						Name: resourceName("crawler-pod"),
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						Image:   instance.Spec.Image,
						Command: []string{"monocle", "crawler"},
						EnvFrom: []corev1.EnvFromSource{
							{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: apiSecretName,
									},
								},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name:  "MONOCLE_PUBLIC_URL",
								Value: monocleAPIInternalURL,
							},
						},
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: int32(crawlerPort),
							},
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt(crawlerPort),
								},
							},
							TimeoutSeconds:   30,
							FailureThreshold: 6,
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      crawlerConfigMapVolumeName,
								ReadOnly:  true,
								MountPath: "/etc/monocle",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: crawlerConfigMapVolumeName,
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									// We use the API config for now
									Name: apiConfigMapName,
								},
							},
						},
					},
				},
			},
		}
		if err := ctrl_util.SetControllerReference(owner, &crawlerDeployment, r.Scheme); err != nil {
			logger.Error(err, "Unable to set controller reference", "name", crawlerDeploymentName)
			return reconcileLater(err)
		}
		logger.Info("Creating Deployment", "name", crawlerDeploymentName)
		// Create the resource
		if err := r.Create(ctx, &crawlerDeployment); err != nil {
			logger.Error(err, "Unable to create deployment", "name", crawlerDeploymentName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Error(err, "Unable to get resource", "name", crawlerDeploymentName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", crawlerDeploymentName)

		// Handle Monocle image update
		if currentCrawlerDeployment.Spec.Template.Annotations["monocleImage"] != instance.Spec.Image {
			currentCrawlerDeployment.Spec.Template.Annotations["monocleImage"] = instance.Spec.Image
			currentCrawlerDeployment.Spec.Template.Spec.Containers[0].Image = instance.Spec.Image
			// Update Deployment Resource to set the new container image
			logger.Info("Start a rollout of the crawler due to container image change")
			err := r.Update(ctx, &currentCrawlerDeployment)
			if err != nil {
				logger.Error(err, "Unable to update resource", "name", apiDeploymentName)
			}
			return reconcileLater(err)
		}

		// Check pod template Annotation secretsVersion
		err := r.rollOutWhenApiSecretsChange(ctx, logger, currentCrawlerDeployment, apiSecretsVersion)
		if err != nil {
			logger.Info("Unable to update deployment annotations", "name", crawlerDeploymentName)
			reconcileLater(err)
		}
	}

	////////////////////////////////////////////////////////
	//           Setting resources statuses               //
	////////////////////////////////////////////////////////

	instance.Status = monoclev1alpha1.MonocleStatus{
		Elastic: serviceStatusConverter(elasticSearchReady()),
		Api:     serviceStatusConverter(isDeploymentReady(&currentApiDeployment)),
		Crawler: serviceStatusConverter(isDeploymentReady(&currentCrawlerDeployment)),
	}

	status := r.Status()
	err = status.Update(ctx, &instance)
	if err != nil {
		reconcileLater(err)
	}

	////////////////////////////////////////////////////////
	//           Checking resources statuses              //
	////////////////////////////////////////////////////////

	// Continue reconcile until elastic is ready
	if !elasticSearchReady() {
		logger.Info("monocle-elastic is not ready")
		reconcileLater(nil)
	}
	// Continue reconcile until api is ready
	if !isDeploymentReady(&currentApiDeployment) {
		logger.Info("monocle-api is not ready")
		return reconcileLater(nil)
	}
	// Continue reconcile until crawler is ready
	if !isDeploymentReady(&currentCrawlerDeployment) {
		logger.Info("monocle-crawler is not ready")
		return reconcileLater(nil)
	}

	////////////////////////////////////////////////////////
	//  We reached here then the reconcile is completed   //
	////////////////////////////////////////////////////////

	// TODO: Set proper instance status
	logger.Info("monocle operand reconcile terminated")
	return stopReconcile()
}

// SetupWithManager sets up the controller with the Manager.
func (r *MonocleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monoclev1alpha1.Monocle{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
