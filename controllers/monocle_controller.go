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
	netv1 "k8s.io/api/networking/v1"
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

func triggerUpdateIdentsJob(r *MonocleReconciler, ctx context.Context, instance monoclev1alpha1.Monocle, namespace string, logger logr.Logger, elasticUrlEnvVar corev1.EnvVar, apiConfigMapName string) error {

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
							Image:   "quay.io/change-metrics/monocle:1.8.0",
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

// https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#deployment-status
func isDeploymentReady(cond appsv1.DeploymentCondition) bool {
	switch cond.Status {
	case corev1.ConditionTrue:
		switch cond.Type {
		case appsv1.DeploymentAvailable:
			return true
		case appsv1.DeploymentProgressing:
			switch cond.Reason {
			case "NewReplicaSetAvailable", "FoundNewReplicaSet", "ReplicaSetUpdated":
				return true
			default:
				return false
			}
		default:
			return false
		}
	default:
		return false
	}
}

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *MonocleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		logger         = log.FromContext(ctx)
		reconcileLater = func(err error) (
			ctrl.Result, error) {
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
		stopReconcile = func() (
			ctrl.Result, error) {
			return ctrl.Result{}, nil
		}
		instance                 = monoclev1alpha1.Monocle{}
		runAsNonRoot             = true
		allowPrivilegeEscalation = false
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

	// Utility to build a name prepended with the Monocle instance's name
	resourceName := func(rName string) string { return instance.Name + "-" + rName }

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
			Namespace: req.Namespace,
		},
	}
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: elasticServiceName, Namespace: req.Namespace}, &elasticService)
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
		if err := ctrl_util.SetControllerReference(&instance, &elasticService, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", elasticServiceName)
			return reconcileLater(err)
		}
		logger.Info("Creating Service", "name", elasticServiceName)
		if err := r.Create(ctx, &elasticService); err != nil {
			logger.Info("Unable to create service", "name", elasticService)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", elasticServiceName)
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
			Namespace: req.Namespace,
		},
	}
	elasticReplicasCount := int32(1)
	// TODO How to handle this ? Should it be expose via the CRD ?
	elasticPVCStorageClassName := "topolvm-provisioner"
	// TODO How to handle this ? Should it be expose via the CRD ?
	elasticPVCStorageQuantity := resource.NewQuantity(1*10^9, resource.DecimalSI)

	elasticSearchReady := func() bool {
		return elasticReplicasCount == elasticStatefulSet.Status.ReadyReplicas
	}

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: elasticStatefulSetName, Namespace: req.Namespace}, &elasticStatefulSet)
	if err != nil && k8s_errors.IsNotFound(err) {
		elasticDataVolumeName := resourceName("elastic-data-volume")
		// Once created StatefulSet selector is immutable
		elasticStatefulSet.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: elasticMatchLabels,
		}
		// Set replicas count
		elasticStatefulSet.Spec.Replicas = &elasticReplicasCount
		// Set the volume claim templates
		elasticStatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      elasticDataVolumeName,
					Namespace: req.Namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: &elasticPVCStorageClassName,
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": *elasticPVCStorageQuantity,
						},
					},
				},
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
		if err := ctrl_util.SetControllerReference(&instance, &elasticStatefulSet, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", elasticStatefulSetName)
			return reconcileLater(err)
		}
		// Create the resource
		logger.Info("Creating StatefulSet", "name", elasticStatefulSetName)
		if err := r.Create(ctx, &elasticStatefulSet); err != nil {
			logger.Info("Unable to create deployment", "name", elasticStatefulSetName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", elasticStatefulSetName)
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
			Namespace: req.Namespace},
	}
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiSecretName, Namespace: req.Namespace}, &apiSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		apiSecret.Data = apiSecretData
		if err := ctrl_util.SetControllerReference(&instance, &apiSecret, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", apiSecretName)
			return reconcileLater(err)
		}
		logger.Info("Creating secret", "name", apiSecretName)
		// Create the resource
		if err := r.Create(ctx, &apiSecret); err != nil {
			logger.Info("Unable to create secret", "name", apiSecretName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", apiSecretName)
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
			Namespace: req.Namespace},
	}

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiConfigMapName, Namespace: req.Namespace}, &apiConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		apiConfigMap.Data = apiConfigMapData
		if err := ctrl_util.SetControllerReference(&instance, &apiConfigMap, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", apiConfigMapName)
			return reconcileLater(err)
		}
		logger.Info("Creating ConfigMap", "name", apiConfigMapName)
		// Create the secret
		if err := r.Create(ctx, &apiConfigMap); err != nil {
			logger.Info("Unable to create configMap", "name", apiConfigMap)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", apiConfigMapName)
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
			Namespace: req.Namespace,
		},
	}
	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiServiceName, Namespace: req.Namespace}, &apiService)
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
		if err := ctrl_util.SetControllerReference(&instance, &apiService, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", apiServiceName)
			return reconcileLater(err)
		}
		logger.Info("Creating Service", "name", apiServiceName)
		if err := r.Create(ctx, &apiService); err != nil {
			logger.Info("Unable to create service", "name", apiService)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", apiServiceName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiServiceName)
	}

	// Handle API deployment //
	///////////////////////////

	apiDeploymentName := resourceName("api")
	apiDeployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiDeploymentName,
			Namespace: req.Namespace,
		},
	}
	apiReplicasCount := int32(1)

	// Func to get the last condition of the Monocle API Deployment instance
	apiDeploymentLastCondition := func() appsv1.DeploymentCondition {
		if len(apiDeployment.Status.Conditions) > 0 {
			return apiDeployment.Status.Conditions[0]
		} else {
			return appsv1.DeploymentCondition{}
		}
	}

	monoclePublicURL := "https://" + instance.Spec.MonoclePublicFQDN
	logger.Info("Monocle public URL set to", "url", monoclePublicURL)

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiDeploymentName, Namespace: req.Namespace}, &apiDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
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
						Image:   "quay.io/change-metrics/monocle:1.8.0",
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
		if err := ctrl_util.SetControllerReference(&instance, &apiDeployment, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", apiDeploymentName)
			return reconcileLater(err)
		}
		logger.Info("Creating Deployment", "name", apiDeploymentName)
		// Create the resource
		if err := r.Create(ctx, &apiDeployment); err != nil {
			logger.Info("Unable to create deployment", "name", apiDeploymentName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", apiDeploymentName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiDeploymentName)

		// Check pod template Annotation secretsVersion
		err := r.rollOutWhenApiSecretsChange(ctx, logger, apiDeployment, apiSecretsVersion)
		if err != nil {
			logger.Info("Unable to update spec deployment annotations", "name", apiDeploymentName)
			reconcileLater(err)
		}

		// Check if Deployment Pod Annotation for ConfigMap resource version was updated
		previousVersion := apiDeployment.Annotations["apiConfigVersion"]
		if previousVersion != apiConfigVersion {

			logger.Info("Start the update-idents jobs because of api configMap update",
				"name", apiDeployment.Name,
				"previous configmap version", previousVersion,
				"new configmap version", apiConfigVersion)
			apiDeployment.Annotations["apiConfigVersion"] = apiConfigVersion
			// Update Deployment Resource to set the new configMap resource version
			err := r.Update(ctx, &apiDeployment)
			if err != nil {
				return reconcileLater(err)
			}
			// Trigger the job
			err = triggerUpdateIdentsJob(r, ctx, instance, req.Namespace, logger, elasticUrlEnvVar, apiConfigMapName)
			if err != nil {
				logger.Info("Unable to trigger update-idents", "name", err)
				reconcileLater(err)
			}
		}
	}

	// Handle ingress for api //
	////////////////////////////

	var apiIngressName = resourceName("api-ingress")
	var apiIngress netv1.Ingress
	var annotations = make(map[string]string)
	annotations["route.openshift.io/termination"] = "edge"
	apiIngress = netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        apiIngressName,
			Namespace:   req.Namespace,
			Annotations: annotations,
		},
	}

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: apiIngressName, Namespace: req.Namespace}, &apiIngress)
	if err != nil && k8s_errors.IsNotFound(err) {
		pt := netv1.PathTypePrefix
		apiIngress.Spec = netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: instance.Spec.MonoclePublicFQDN,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									PathType: &pt,
									Path:     "/",
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: apiSecretName,
											Port: netv1.ServiceBackendPort{
												Number: int32(apiPort),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		if err := ctrl_util.SetControllerReference(&instance, &apiIngress, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", apiIngressName)
			return reconcileLater(err)
		}
		logger.Info("Creating Ingress", "name", apiIngressName)
		if err := r.Create(ctx, &apiIngress); err != nil {
			logger.Info("Unable to create ingress", "name", apiIngress)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", apiIngressName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", apiIngressName)
	}

	////////////////////////////////////////////////////////
	//   Handle the Monocle Crawler Deployment instance   //
	////////////////////////////////////////////////////////

	monocleAPIInternalURL := "http://" + apiServiceName + ":" + strconv.Itoa(apiPort)

	crawlerDeploymentName := resourceName("crawler")
	crawlerDeployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crawlerDeploymentName,
			Namespace: req.Namespace,
		},
	}
	crawlerReplicasCount := int32(1)
	crawlerPort := 9001
	crawlerMatchLabels := map[string]string{
		"app":  "monocle",
		"tier": "crawler",
	}
	// Func to get the last condition of the Monocle crawler Deployment instance
	crawlerDeploymentLastCondition := func() appsv1.DeploymentCondition {
		if len(crawlerDeployment.Status.Conditions) > 0 {
			return crawlerDeployment.Status.Conditions[0]
		} else {
			return appsv1.DeploymentCondition{}
		}
	}

	err = r.Client.Get(
		ctx, client.ObjectKey{Name: crawlerDeploymentName, Namespace: req.Namespace}, &crawlerDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
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
						Image:   "quay.io/change-metrics/monocle:1.8.0",
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
		if err := ctrl_util.SetControllerReference(&instance, &crawlerDeployment, r.Scheme); err != nil {
			logger.Info("Unable to set controller reference", "name", crawlerDeploymentName)
			return reconcileLater(err)
		}
		logger.Info("Creating Deployment", "name", crawlerDeploymentName)
		// Create the resource
		if err := r.Create(ctx, &crawlerDeployment); err != nil {
			logger.Info("Unable to create deployment", "name", crawlerDeploymentName)
			return reconcileLater(err)
		}
	} else if err != nil {
		// Handle the unexpected err
		logger.Info("Unable to get resource", "name", crawlerDeploymentName)
		return reconcileLater(err)
	} else {
		// Eventually handle resource update
		logger.Info("Resource fetched successfuly", "name", crawlerDeploymentName)

		// Check pod template Annotation secretsVersion
		err := r.rollOutWhenApiSecretsChange(ctx, logger, crawlerDeployment, apiSecretsVersion)
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
		Api:     serviceStatusConverter(isDeploymentReady(apiDeploymentLastCondition())),
		Crawler: serviceStatusConverter(isDeploymentReady(crawlerDeploymentLastCondition())),
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
	if isDeploymentReady(apiDeploymentLastCondition()) == false {
		logger.Info("monocle-api is not ready", "condition", apiDeploymentLastCondition())
		return reconcileLater(nil)
	}
	// Continue reconcile until crawler is ready
	if isDeploymentReady(crawlerDeploymentLastCondition()) == false {
		logger.Info("monocle-crawler is not ready", "condition", crawlerDeploymentLastCondition())
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
