/*
Copyright 2023.

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
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bigdatav1alpha1 "github.com/kubernetesbigdataeg/ranger-operator/api/v1alpha1"
)

const rangerFinalizer = "bigdata.kubernetesbigdataeg.org/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableRanger represents the status of the Deployment reconciliation
	typeAvailableRanger = "Available"
	// typeDegradedRanger represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedRanger = "Degraded"
)

// RangerReconciler reconciles a Ranger object
type RangerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=rangers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=rangers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bigdata.kubernetesbigdataeg.org,resources=rangers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=configmaps;services,verbs=get;list;create;watch
//+kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *RangerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	//
	// 1. Control-loop: checking if Ranger CR exists
	//
	// Fetch the Ranger instance
	// The purpose is check if the Custom Resource for the Kind Ranger
	// is applied on the cluster if not we return nil to stop the reconciliation
	ranger := &bigdatav1alpha1.Ranger{}
	err := r.Get(ctx, req.NamespacedName, ranger)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("ranger resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get ranger")
		return ctrl.Result{}, err
	}

	//
	// 2. Control-loop: Status to Unknown
	//
	// Let's just set the status as Unknown when no status are available
	if ranger.Status.Conditions == nil || len(ranger.Status.Conditions) == 0 {
		meta.SetStatusCondition(&ranger.Status.Conditions, metav1.Condition{Type: typeAvailableRanger, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, ranger); err != nil {
			log.Error(err, "Failed to update Ranger status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the ranger Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, ranger); err != nil {
			log.Error(err, "Failed to re-fetch ranger")
			return ctrl.Result{}, err
		}
	}

	//
	// 3. Control-loop: Let's add a finalizer
	//
	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(ranger, rangerFinalizer) {
		log.Info("Adding Finalizer for Ranger")
		if ok := controllerutil.AddFinalizer(ranger, rangerFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, ranger); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	//
	// 4. Control-loop: Instance marked for deletion
	//
	// Check if the Ranger instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isRangerMarkedToBeDeleted := ranger.GetDeletionTimestamp() != nil
	if isRangerMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(ranger, rangerFinalizer) {
			log.Info("Performing Finalizer Operations for Ranger before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&ranger.Status.Conditions, metav1.Condition{Type: typeDegradedRanger,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", ranger.Name)})

			if err := r.Status().Update(ctx, ranger); err != nil {
				log.Error(err, "Failed to update Ranger status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForRanger(ranger)

			// TODO(user): If you add operations to the doFinalizerOperationsForRanger method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the ranger Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, ranger); err != nil {
				log.Error(err, "Failed to re-fetch ranger")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&ranger.Status.Conditions, metav1.Condition{Type: typeDegradedRanger,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", ranger.Name)})

			if err := r.Status().Update(ctx, ranger); err != nil {
				log.Error(err, "Failed to update Ranger status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for Ranger after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(ranger, rangerFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for Ranger")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, ranger); err != nil {
				log.Error(err, "Failed to remove finalizer for Ranger")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	//
	// 5. Control-loop: Let's deploy/ensure our managed resources for Ranger
	// - Service NodePort,
	// - Deployment
	//
	// Check if the deployment already exists, if not create a new one
	// Service
	serviceFound := &corev1.Service{}
	if err := r.ensureResource(ctx, ranger, r.serviceForRanger, serviceFound, "ranger-svc", "Service"); err != nil {
		return ctrl.Result{}, err
	}

	// Deployment
	deploymentFound := &appsv1.Deployment{}
	if err := r.ensureResource(ctx, ranger, r.deploymentForRanger, deploymentFound, ranger.Name, "Deployment"); err != nil {
		return ctrl.Result{}, err
	}

	//
	// 6. Control-loop: Check the number of replicas
	//
	// The CRD API is defining that the Ranger type, have a RangerSpec.Size field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	size := ranger.Spec.Size
	if *deploymentFound.Spec.Replicas != size {
		deploymentFound.Spec.Replicas = &size
		if err = r.Update(ctx, deploymentFound); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", deploymentFound.Namespace, "Deployment.Name", deploymentFound.Name)

			// Re-fetch the ranger Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, ranger); err != nil {
				log.Error(err, "Failed to re-fetch ranger")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&ranger.Status.Conditions, metav1.Condition{Type: typeAvailableRanger,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", ranger.Name, err)})

			if err := r.Status().Update(ctx, ranger); err != nil {
				log.Error(err, "Failed to update Ranger status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&ranger.Status.Conditions, metav1.Condition{Type: typeAvailableRanger,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", ranger.Name, size)})

	if err := r.Status().Update(ctx, ranger); err != nil {
		log.Error(err, "Failed to update Ranger status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// finalizeRanger will perform the required operations before delete the CR.
func (r *RangerReconciler) doFinalizerOperationsForRanger(cr *bigdatav1alpha1.Ranger) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *RangerReconciler) serviceForRanger(Ranger *bigdatav1alpha1.Ranger, resourceName string) (client.Object, error) {

	labels := labelsForRanger(Ranger.Name)
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: Ranger.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:     "svc-admin-http",
				Port:     6080,
				NodePort: 30002,
			}},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	if err := ctrl.SetControllerReference(Ranger, s, r.Scheme); err != nil {
		return nil, err
	}

	return s, nil
}

// deploymentForRanger returns a Ranger Deployment object
func (r *RangerReconciler) deploymentForRanger(ranger *bigdatav1alpha1.Ranger, resourceName string) (client.Object, error) {

	labels := labelsForRanger(ranger.Name)

	replicas := ranger.Spec.Size

	image, err := imageForRanger()
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: ranger.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "ranger",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env: []corev1.EnvVar{
								{
									Name:  "DB_FLAVOR",
									Value: "POSTGRES",
								},
								{
									Name:  "DB_HOST",
									Value: "postgres-sample-0.postgres-svc.default.svc.cluster.local",
								},
								{
									Name:  "DB_PORT",
									Value: "5432",
								},
								{
									Name:  "SOLR_HOST",
									Value: "solr-sample-0.solrcluster.default.svc.cluster.local,solr-sample-1.solrcluster.default.svc.cluster.local",
								},
								{
									Name:  "SOLR_PORT",
									Value: "8983",
								},
								{
									Name:  "SOLR_USER",
									Value: "solr",
								},
								{
									Name:  "SOLR_PASSWORD",
									Value: "solr",
								},
								{
									Name:  "ZOOKEEPERS",
									Value: "zk-0.zk-hs.default.svc.cluster.local:2181,zk-1.zk-hs.default.svc.cluster.local:2181,zk-2.zk-hs.default.svc.cluster.local:2181",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "svc-admin-http",
									ContainerPort: 6080,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "svc-admin-https",
									ContainerPort: 6182,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
	}

	return deployment, nil
}

// labelsForRanger returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForRanger(name string) map[string]string {
	var imageTag string
	image, err := imageForRanger()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{
		"app.kubernetes.io/name":       "Ranger",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "ranger-operator",
		"app.kubernetes.io/created-by": "controller-manager",
		"app":                          "ranger",
	}
}

// imageForRanger gets the Operand image which is managed by this controller
// from the RANGER_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForRanger() (string, error) {
	/*var imageEnvVar = "RANGER_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}*/
	image := "docker.io/kubernetesbigdataeg/ranger:2.4.0-1"
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *RangerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bigdatav1alpha1.Ranger{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

func (r *RangerReconciler) ensureResource(ctx context.Context, ranger *bigdatav1alpha1.Ranger, createResourceFunc func(*bigdatav1alpha1.Ranger, string) (client.Object, error), foundResource client.Object, resourceName string, resourceType string) error {
	log := log.FromContext(ctx)
	err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: ranger.Namespace}, foundResource)
	if err != nil && apierrors.IsNotFound(err) {
		resource, err := createResourceFunc(ranger, resourceName)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to define new %s resource for Ranger", resourceType))

			// The following implementation will update the status
			meta.SetStatusCondition(&ranger.Status.Conditions, metav1.Condition{Type: typeAvailableRanger,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create %s for the custom resource (%s): (%s)", resourceType, ranger.Name, err)})

			if err := r.Status().Update(ctx, ranger); err != nil {
				log.Error(err, "Failed to update Ranger status")
				return err
			}

			return err
		}

		log.Info(fmt.Sprintf("Creating a new %s", resourceType),
			fmt.Sprintf("%s.Namespace", resourceType), resource.GetNamespace(), fmt.Sprintf("%s.Name", resourceType), resource.GetName())

		if err = r.Create(ctx, resource); err != nil {
			log.Error(err, fmt.Sprintf("Failed to create new %s", resourceType),
				fmt.Sprintf("%s.Namespace", resourceType), resource.GetNamespace(), fmt.Sprintf("%s.Name", resourceType), resource.GetName())
			return err
		}

		time.Sleep(5 * time.Second)

		if err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: ranger.Namespace}, foundResource); err != nil {
			log.Error(err, fmt.Sprintf("Failed to get newly created %s", resourceType))
			return err
		}

	} else if err != nil {
		log.Error(err, fmt.Sprintf("Failed to get %s", resourceType))
		return err
	}

	return nil
}
