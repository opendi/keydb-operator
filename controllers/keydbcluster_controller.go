/*
Copyright 2024.

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
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keydbv1alpha1 "github.com/your-org/keydb-operator/api/v1alpha1"
	"github.com/your-org/keydb-operator/internal/health"
	"github.com/your-org/keydb-operator/internal/resources"
	"github.com/your-org/keydb-operator/internal/upgrade"
	"github.com/your-org/keydb-operator/internal/validation"
)

// KeyDBClusterReconciler reconciles a KeyDBCluster object
type KeyDBClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	keydbClusterFinalizer = "keydb.io/finalizer"
)

//+kubebuilder:rbac:groups=keydb.io,resources=keydbclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=keydb.io,resources=keydbclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=keydb.io,resources=keydbclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KeyDBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the KeyDBCluster instance
	keydbCluster := &keydbv1alpha1.KeyDBCluster{}
	err := r.Get(ctx, req.NamespacedName, keydbCluster)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("KeyDBCluster resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get KeyDBCluster")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if keydbCluster.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, keydbCluster)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(keydbCluster, keydbClusterFinalizer) {
		controllerutil.AddFinalizer(keydbCluster, keydbClusterFinalizer)
		return ctrl.Result{}, r.Update(ctx, keydbCluster)
	}

	// Set default values
	r.setDefaults(keydbCluster)

	// Validate configuration
	validator := validation.NewValidator()
	if err := validator.ValidateKeyDBCluster(keydbCluster); err != nil {
		logger.Error(err, "Configuration validation failed")
		// Update status with validation error
		keydbCluster.Status.Phase = keydbv1alpha1.FailedPhase
		r.Status().Update(ctx, keydbCluster)
		return ctrl.Result{}, err
	}

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, keydbCluster); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile Services
	if err := r.reconcileServices(ctx, keydbCluster); err != nil {
		logger.Error(err, "Failed to reconcile Services")
		return ctrl.Result{}, err
	}

	// Reconcile PodDisruptionBudget
	if err := r.reconcilePodDisruptionBudget(ctx, keydbCluster); err != nil {
		logger.Error(err, "Failed to reconcile PodDisruptionBudget")
		return ctrl.Result{}, err
	}

	// Check for upgrades
	upgradeNeeded, err := r.checkUpgradeNeeded(ctx, keydbCluster)
	if err != nil {
		logger.Error(err, "Failed to check upgrade status")
		return ctrl.Result{}, err
	}

	if upgradeNeeded {
		// Perform rolling upgrade
		if err := r.performRollingUpgrade(ctx, keydbCluster); err != nil {
			logger.Error(err, "Failed to perform rolling upgrade")
			return ctrl.Result{}, err
		}
		// Requeue quickly during upgrades
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Reconcile StatefulSet
	if err := r.reconcileStatefulSet(ctx, keydbCluster); err != nil {
		logger.Error(err, "Failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}

	// Perform health checks
	if err := r.performHealthChecks(ctx, keydbCluster); err != nil {
		logger.Error(err, "Failed to perform health checks")
		// Don't return error for health check failures, just log
	}

	// Update status
	if err := r.updateStatus(ctx, keydbCluster); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue after 30 seconds for status updates
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *KeyDBClusterReconciler) handleDeletion(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(keydbCluster, keydbClusterFinalizer) {
		// Perform cleanup logic here if needed
		logger.Info("Cleaning up KeyDBCluster resources")

		// Remove finalizer
		controllerutil.RemoveFinalizer(keydbCluster, keydbClusterFinalizer)
		return ctrl.Result{}, r.Update(ctx, keydbCluster)
	}

	return ctrl.Result{}, nil
}

func (r *KeyDBClusterReconciler) setDefaults(keydbCluster *keydbv1alpha1.KeyDBCluster) {
	if keydbCluster.Spec.Image == "" {
		keydbCluster.Spec.Image = "eqalpha/keydb:latest"
	}
	if keydbCluster.Spec.Replicas == 0 {
		keydbCluster.Spec.Replicas = 3
	}
	if keydbCluster.Spec.Storage == nil {
		keydbCluster.Spec.Storage = &keydbv1alpha1.StorageConfig{
			Size: "10Gi",
		}
	}
	if keydbCluster.Spec.Service == nil {
		keydbCluster.Spec.Service = &keydbv1alpha1.ServiceConfig{
			Type: "ClusterIP",
			Port: 6379,
		}
	}
	if keydbCluster.Spec.Config == nil {
		keydbCluster.Spec.Config = &keydbv1alpha1.KeyDBConfig{
			Persistence: true,
		}
	}
}

func (r *KeyDBClusterReconciler) reconcileConfigMap(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	configMap := resources.NewConfigMap(keydbCluster)
	
	// Set controller reference
	if err := controllerutil.SetControllerReference(keydbCluster, configMap, r.Scheme); err != nil {
		return err
	}

	// Check if ConfigMap exists
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// Update if needed
	if found.Data["keydb.conf"] != configMap.Data["keydb.conf"] {
		found.Data = configMap.Data
		return r.Update(ctx, found)
	}

	return nil
}

func (r *KeyDBClusterReconciler) reconcileServices(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	// Headless service
	headlessService := resources.NewHeadlessService(keydbCluster)
	if err := controllerutil.SetControllerReference(keydbCluster, headlessService, r.Scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: headlessService.Name, Namespace: headlessService.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, headlessService); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Client service
	clientService := resources.NewClientService(keydbCluster)
	if err := controllerutil.SetControllerReference(keydbCluster, clientService, r.Scheme); err != nil {
		return err
	}

	found = &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: clientService.Name, Namespace: clientService.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, clientService)
	} else if err != nil {
		return err
	}

	return nil
}

func (r *KeyDBClusterReconciler) reconcileStatefulSet(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	statefulSet := resources.NewStatefulSet(keydbCluster)
	
	// Set controller reference
	if err := controllerutil.SetControllerReference(keydbCluster, statefulSet, r.Scheme); err != nil {
		return err
	}

	// Check if StatefulSet exists
	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// Update if needed (simplified - in production you'd want more sophisticated update logic)
	if found.Spec.Replicas == nil || *found.Spec.Replicas != *statefulSet.Spec.Replicas {
		found.Spec.Replicas = statefulSet.Spec.Replicas
		return r.Update(ctx, found)
	}

	return nil
}

func (r *KeyDBClusterReconciler) updateStatus(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	// Get StatefulSet to check status
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-keydb", keydbCluster.Name),
		Namespace: keydbCluster.Namespace,
	}, statefulSet)
	if err != nil {
		return err
	}

	// Update status based on StatefulSet
	keydbCluster.Status.Replicas = statefulSet.Status.Replicas
	keydbCluster.Status.ReadyReplicas = statefulSet.Status.ReadyReplicas

	// Determine phase
	if keydbCluster.Status.ReadyReplicas == 0 {
		keydbCluster.Status.Phase = keydbv1alpha1.PendingPhase
	} else if keydbCluster.Status.ReadyReplicas == keydbCluster.Status.Replicas {
		keydbCluster.Status.Phase = keydbv1alpha1.RunningPhase
	} else {
		keydbCluster.Status.Phase = keydbv1alpha1.PendingPhase
	}

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "NotReady",
		Message:            "KeyDB cluster is not ready",
		LastTransitionTime: metav1.Now(),
	}

	if keydbCluster.Status.Phase == keydbv1alpha1.RunningPhase {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Ready"
		condition.Message = "KeyDB cluster is ready"
	}

	// Update or add condition
	keydbCluster.Status.Conditions = []metav1.Condition{condition}

	return r.Status().Update(ctx, keydbCluster)
}

func (r *KeyDBClusterReconciler) reconcilePodDisruptionBudget(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.PodDisruptionBudget == nil {
		return nil // PDB not configured
	}

	pdb := resources.NewPodDisruptionBudget(keydbCluster)
	if err := controllerutil.SetControllerReference(keydbCluster, pdb, r.Scheme); err != nil {
		return err
	}

	found := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: pdb.Name, Namespace: pdb.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, pdb)
	} else if err != nil {
		return err
	}

	return nil
}

func (r *KeyDBClusterReconciler) checkUpgradeNeeded(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) (bool, error) {
	// Get current StatefulSet
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-keydb", keydbCluster.Name),
		Namespace: keydbCluster.Namespace,
	}, statefulSet)
	if err != nil {
		return false, err
	}

	// Check if image needs to be updated
	currentImage := ""
	if len(statefulSet.Spec.Template.Spec.Containers) > 0 {
		currentImage = statefulSet.Spec.Template.Spec.Containers[0].Image
	}

	return currentImage != keydbCluster.Spec.Image, nil
}

func (r *KeyDBClusterReconciler) performRollingUpgrade(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	logger := log.FromContext(ctx)
	logger.Info("Starting rolling upgrade", "targetImage", keydbCluster.Spec.Image)

	// Initialize upgrade status if not present
	if keydbCluster.Status.UpgradeStatus == nil {
		keydbCluster.Status.UpgradeStatus = &keydbv1alpha1.UpgradeStatus{
			TargetImage: keydbCluster.Spec.Image,
			Phase:       "Starting",
			StartTime:   &metav1.Time{Time: time.Now()},
		}
		if err := r.Status().Update(ctx, keydbCluster); err != nil {
			return err
		}
	}

	// Use upgrade manager for safe rolling upgrade
	upgradeManager := upgrade.NewManager(r.Client, r.Scheme)
	return upgradeManager.PerformRollingUpgrade(ctx, keydbCluster)
}

func (r *KeyDBClusterReconciler) performHealthChecks(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	healthChecker := health.NewChecker(r.Client)
	clusterHealth, err := healthChecker.CheckClusterHealth(ctx, keydbCluster)
	if err != nil {
		return err
	}

	// Update health status
	keydbCluster.Status.Health = clusterHealth
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KeyDBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keydbv1alpha1.KeyDBCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
