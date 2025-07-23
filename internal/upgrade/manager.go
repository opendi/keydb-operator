package upgrade

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	keydbv1alpha1 "github.com/opendi/keydb-operator/api/v1alpha1"
)

// Manager handles rolling upgrades for KeyDB clusters
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManager creates a new upgrade manager
func NewManager(client client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: client,
		scheme: scheme,
	}
}

// PerformRollingUpgrade performs a safe rolling upgrade of the KeyDB cluster
func (m *Manager) PerformRollingUpgrade(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	logger := log.FromContext(ctx)
	logger.Info("Performing rolling upgrade", "cluster", keydbCluster.Name, "targetImage", keydbCluster.Spec.Image)

	// Get current StatefulSet
	statefulSet := &appsv1.StatefulSet{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-keydb", keydbCluster.Name),
		Namespace: keydbCluster.Namespace,
	}, statefulSet)
	if err != nil {
		return err
	}

	// Validate upgrade strategy
	strategy := "RollingUpdate"
	if keydbCluster.Spec.Upgrade != nil && keydbCluster.Spec.Upgrade.Strategy != "" {
		strategy = keydbCluster.Spec.Upgrade.Strategy
	}

	switch strategy {
	case "RollingUpdate":
		return m.performRollingUpdate(ctx, keydbCluster, statefulSet)
	case "Recreate":
		return m.performRecreateUpdate(ctx, keydbCluster, statefulSet)
	default:
		return fmt.Errorf("unsupported upgrade strategy: %s", strategy)
	}
}

func (m *Manager) performRollingUpdate(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, statefulSet *appsv1.StatefulSet) error {
	logger := log.FromContext(ctx)

	// Update upgrade status
	keydbCluster.Status.UpgradeStatus.Phase = "InProgress"
	keydbCluster.Status.UpgradeStatus.TotalPods = *statefulSet.Spec.Replicas
	if err := m.updateStatus(ctx, keydbCluster); err != nil {
		return err
	}

	// Determine upgrade order based on mode
	var upgradeOrder []int32
	if keydbCluster.Spec.Mode == keydbv1alpha1.ClusterMode {
		// For cluster mode, upgrade replicas first, then masters
		upgradeOrder = m.getClusterModeUpgradeOrder(keydbCluster)
	} else {
		// For multi-master mode, upgrade one at a time
		upgradeOrder = m.getMultiMasterUpgradeOrder(keydbCluster)
	}

	// Perform rolling upgrade pod by pod
	for _, podIndex := range upgradeOrder {
		if err := m.upgradePod(ctx, keydbCluster, statefulSet, podIndex); err != nil {
			keydbCluster.Status.UpgradeStatus.Phase = "Failed"
			if updateErr := m.updateStatus(ctx, keydbCluster); updateErr != nil {
				logger.Error(updateErr, "Failed to update status after upgrade error")
			}
			return err
		}

		// Update progress
		keydbCluster.Status.UpgradeStatus.UpgradedPods++
		if err := m.updateStatus(ctx, keydbCluster); err != nil {
			return err
		}

		logger.Info("Successfully upgraded pod", "podIndex", podIndex)
	}

	// Mark upgrade as complete
	keydbCluster.Status.UpgradeStatus.Phase = "Completed"
	keydbCluster.Status.CurrentImage = keydbCluster.Spec.Image
	return m.updateStatus(ctx, keydbCluster)
}

func (m *Manager) performRecreateUpdate(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, statefulSet *appsv1.StatefulSet) error {
	logger := log.FromContext(ctx)
	logger.Info("Performing recreate upgrade")

	// Update the StatefulSet image
	statefulSet.Spec.Template.Spec.Containers[0].Image = keydbCluster.Spec.Image
	if err := m.client.Update(ctx, statefulSet); err != nil {
		return err
	}

	// Delete all pods to force recreation
	pods, err := m.getClusterPods(ctx, keydbCluster)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		if err := m.client.Delete(ctx, &pod); err != nil {
			return err
		}
	}

	// Wait for all pods to be recreated and ready
	return m.waitForPodsReady(ctx, keydbCluster, time.Minute*10)
}

func (m *Manager) upgradePod(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, statefulSet *appsv1.StatefulSet, podIndex int32) error {
	logger := log.FromContext(ctx)
	podName := fmt.Sprintf("%s-keydb-%d", keydbCluster.Name, podIndex)

	// Pre-upgrade validation
	if err := m.validatePodBeforeUpgrade(ctx, keydbCluster, podName); err != nil {
		return fmt.Errorf("pre-upgrade validation failed for pod %s: %w", podName, err)
	}

	// Update StatefulSet image if not already updated
	if statefulSet.Spec.Template.Spec.Containers[0].Image != keydbCluster.Spec.Image {
		statefulSet.Spec.Template.Spec.Containers[0].Image = keydbCluster.Spec.Image
		if err := m.client.Update(ctx, statefulSet); err != nil {
			return err
		}
	}

	// Delete the pod to trigger recreation
	pod := &corev1.Pod{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: keydbCluster.Namespace,
	}, pod)
	if err != nil {
		return err
	}

	if err := m.client.Delete(ctx, pod); err != nil {
		return err
	}

	// Wait for pod to be recreated and ready
	timeout := time.Second * 300 // 5 minutes
	if keydbCluster.Spec.Upgrade != nil && keydbCluster.Spec.Upgrade.ValidationTimeoutSeconds > 0 {
		timeout = time.Second * time.Duration(keydbCluster.Spec.Upgrade.ValidationTimeoutSeconds)
	}

	if err := m.waitForPodReady(ctx, keydbCluster, podName, timeout); err != nil {
		return err
	}

	// Post-upgrade validation
	if err := m.validatePodAfterUpgrade(ctx, keydbCluster, podName); err != nil {
		return fmt.Errorf("post-upgrade validation failed for pod %s: %w", podName, err)
	}

	logger.Info("Pod upgrade completed successfully", "pod", podName)
	return nil
}

func (m *Manager) getClusterModeUpgradeOrder(keydbCluster *keydbv1alpha1.KeyDBCluster) []int32 {
	// In cluster mode, upgrade replicas first, then masters
	var order []int32

	if keydbCluster.Spec.Cluster == nil {
		// Default cluster configuration
		for i := int32(0); i < 3; i++ {
			order = append(order, i)
		}
		return order
	}

	shards := keydbCluster.Spec.Cluster.Shards
	replicasPerShard := keydbCluster.Spec.Cluster.ReplicasPerShard

	// First, add all replicas
	for shard := int32(0); shard < shards; shard++ {
		for replica := int32(1); replica <= replicasPerShard; replica++ {
			podIndex := shard*(1+replicasPerShard) + replica
			order = append(order, podIndex)
		}
	}

	// Then, add all masters
	for shard := int32(0); shard < shards; shard++ {
		masterIndex := shard * (1 + replicasPerShard)
		order = append(order, masterIndex)
	}

	return order
}

func (m *Manager) getMultiMasterUpgradeOrder(keydbCluster *keydbv1alpha1.KeyDBCluster) []int32 {
	// In multi-master mode, upgrade one at a time
	var order []int32
	for i := int32(0); i < keydbCluster.Spec.Replicas; i++ {
		order = append(order, i)
	}
	return order
}

func (m *Manager) validatePodBeforeUpgrade(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, podName string) error {
	// Check if pod is healthy before upgrade
	pod := &corev1.Pod{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: keydbCluster.Namespace,
	}, pod)
	if err != nil {
		return err
	}

	// Check if pod is ready
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
			return fmt.Errorf("pod %s is not ready", podName)
		}
	}

	// Additional KeyDB-specific validations could be added here
	return nil
}

func (m *Manager) validatePodAfterUpgrade(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, podName string) error {
	// Validate that the upgraded pod is healthy and functioning
	pod := &corev1.Pod{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      podName,
		Namespace: keydbCluster.Namespace,
	}, pod)
	if err != nil {
		return err
	}

	// Check if pod is ready
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
			return fmt.Errorf("pod %s is not ready after upgrade", podName)
		}
	}

	// Check if the image was updated correctly
	if len(pod.Spec.Containers) > 0 && pod.Spec.Containers[0].Image != keydbCluster.Spec.Image {
		return fmt.Errorf("pod %s image was not updated correctly", podName)
	}

	return nil
}

func (m *Manager) waitForPodReady(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, podName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		pod := &corev1.Pod{}
		err := m.client.Get(ctx, types.NamespacedName{
			Name:      podName,
			Namespace: keydbCluster.Namespace,
		}, pod)
		if err != nil {
			time.Sleep(time.Second * 5)
			continue
		}

		// Check if pod is ready
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return nil
			}
		}

		time.Sleep(time.Second * 5)
	}

	return fmt.Errorf("timeout waiting for pod %s to be ready", podName)
}

func (m *Manager) waitForPodsReady(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		pods, err := m.getClusterPods(ctx, keydbCluster)
		if err != nil {
			time.Sleep(time.Second * 5)
			continue
		}

		allReady := true
		for _, pod := range pods {
			ready := false
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if !ready {
				allReady = false
				break
			}
		}

		if allReady {
			return nil
		}

		time.Sleep(time.Second * 5)
	}

	return fmt.Errorf("timeout waiting for all pods to be ready")
}

func (m *Manager) getClusterPods(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := m.client.List(ctx, podList, &client.ListOptions{
		Namespace: keydbCluster.Namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app":              "keydb",
			"keydb.io/cluster": keydbCluster.Name,
		}),
	})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (m *Manager) updateStatus(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	return m.client.Status().Update(ctx, keydbCluster)
}
