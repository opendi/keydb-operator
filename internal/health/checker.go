package health

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	keydbv1alpha1 "github.com/your-org/keydb-operator/api/v1alpha1"
)

// Checker provides health checking functionality for KeyDB clusters
type Checker struct {
	client    client.Client
	clientset kubernetes.Interface
	config    *rest.Config
}

// NewChecker creates a new health checker
func NewChecker(client client.Client) *Checker {
	// In a real implementation, you'd inject the clientset and config
	return &Checker{
		client: client,
	}
}

// CheckClusterHealth performs comprehensive health checks on the KeyDB cluster
func (c *Checker) CheckClusterHealth(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) (*keydbv1alpha1.ClusterHealth, error) {
	health := &keydbv1alpha1.ClusterHealth{
		Status:        "Unknown",
		LastCheckTime: &metav1.Time{Time: time.Now()},
	}

	// Get all pods for the cluster
	pods, err := c.getClusterPods(ctx, keydbCluster)
	if err != nil {
		health.Status = "Error"
		return health, err
	}

	if len(pods) == 0 {
		health.Status = "NoPodsFound"
		return health, nil
	}

	// Check individual pod health
	healthyPods := 0
	totalMemoryUsage := float64(0)
	totalClients := int32(0)
	maxReplicationLag := int32(0)

	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podHealth, err := c.checkPodHealth(ctx, &pod, keydbCluster)
		if err != nil {
			continue // Skip unhealthy pods
		}

		if podHealth.Healthy {
			healthyPods++
			totalMemoryUsage += podHealth.MemoryUsagePercent
			totalClients += podHealth.ConnectedClients
			if podHealth.ReplicationLag > maxReplicationLag {
				maxReplicationLag = podHealth.ReplicationLag
			}
		}
	}

	// Calculate overall health
	if healthyPods == 0 {
		health.Status = "Critical"
	} else if healthyPods < len(pods)/2 {
		health.Status = "Degraded"
	} else if healthyPods == len(pods) {
		health.Status = "Healthy"
	} else {
		health.Status = "Warning"
	}

	// Set aggregated metrics
	if healthyPods > 0 {
		health.MemoryUsagePercent = totalMemoryUsage / float64(healthyPods)
		health.ConnectedClients = totalClients
		health.ReplicationLagSeconds = maxReplicationLag
	}

	return health, nil
}

// PodHealth represents health metrics for a single pod
type PodHealth struct {
	Healthy              bool
	MemoryUsagePercent   float64
	ConnectedClients     int32
	ReplicationLag       int32
	LastResponseTime     time.Duration
	KeyDBVersion         string
	Role                 string
}

func (c *Checker) checkPodHealth(ctx context.Context, pod *corev1.Pod, keydbCluster *keydbv1alpha1.KeyDBCluster) (*PodHealth, error) {
	health := &PodHealth{
		Healthy: false,
	}

	// Check if pod is ready
	if !c.isPodReady(pod) {
		return health, fmt.Errorf("pod %s is not ready", pod.Name)
	}

	// Execute KeyDB INFO command to get metrics
	info, err := c.executeKeyDBCommand(ctx, pod, "INFO")
	if err != nil {
		return health, err
	}

	// Parse INFO response
	if err := c.parseInfoResponse(info, health); err != nil {
		return health, err
	}

	// Check replication status for cluster mode
	if keydbCluster.Spec.Mode == keydbv1alpha1.ClusterMode {
		clusterInfo, err := c.executeKeyDBCommand(ctx, pod, "CLUSTER INFO")
		if err == nil {
			c.parseClusterInfo(clusterInfo, health)
		}
	}

	// Perform ping test
	start := time.Now()
	_, err = c.executeKeyDBCommand(ctx, pod, "PING")
	if err != nil {
		return health, err
	}
	health.LastResponseTime = time.Since(start)

	health.Healthy = true
	return health, nil
}

func (c *Checker) getClusterPods(ctx context.Context, keydbCluster *keydbv1alpha1.KeyDBCluster) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app":              "keydb",
		"keydb.io/cluster": keydbCluster.Name,
	})

	err := c.client.List(ctx, podList, &client.ListOptions{
		Namespace:     keydbCluster.Namespace,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	return podList.Items, nil
}

func (c *Checker) isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (c *Checker) executeKeyDBCommand(ctx context.Context, pod *corev1.Pod, command string) (string, error) {
	// This is a simplified implementation
	// In a real implementation, you'd use the Kubernetes exec API
	// For now, we'll simulate the response
	
	switch command {
	case "PING":
		return "PONG", nil
	case "INFO":
		return c.simulateInfoResponse(), nil
	case "CLUSTER INFO":
		return c.simulateClusterInfoResponse(), nil
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (c *Checker) simulateInfoResponse() string {
	return `# Server
redis_version:6.2.0
redis_git_sha1:00000000
redis_git_dirty:0
redis_build_id:0
redis_mode:standalone
os:Linux 5.4.0-74-generic x86_64
arch_bits:64
multiplexing_api:epoll
atomicvar_api:atomic-builtin
gcc_version:9.3.0
process_id:1
run_id:1234567890abcdef
tcp_port:6379
uptime_in_seconds:3600
uptime_in_days:0

# Clients
connected_clients:10
client_recent_max_input_buffer:2
client_recent_max_output_buffer:0
blocked_clients:0

# Memory
used_memory:1048576
used_memory_human:1.00M
used_memory_rss:2097152
used_memory_rss_human:2.00M
used_memory_peak:1048576
used_memory_peak_human:1.00M
maxmemory:1073741824
maxmemory_human:1.00G
maxmemory_policy:allkeys-lru
mem_fragmentation_ratio:2.00

# Replication
role:master
connected_slaves:1
slave0:ip=10.244.0.5,port=6379,state=online,offset=1000,lag=1`
}

func (c *Checker) simulateClusterInfoResponse() string {
	return `cluster_state:ok
cluster_slots_assigned:16384
cluster_slots_ok:16384
cluster_slots_pfail:0
cluster_slots_fail:0
cluster_known_nodes:6
cluster_size:3
cluster_current_epoch:6
cluster_my_epoch:2
cluster_stats_messages_sent:1234
cluster_stats_messages_received:1234`
}

func (c *Checker) parseInfoResponse(info string, health *PodHealth) error {
	lines := strings.Split(info, "\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "connected_clients":
				if clients, err := strconv.Atoi(value); err == nil {
					health.ConnectedClients = int32(clients)
				}
			case "used_memory":
				if usedMem, err := strconv.ParseFloat(value, 64); err == nil {
					// Calculate percentage (simplified)
					health.MemoryUsagePercent = (usedMem / 1073741824) * 100 // Assume 1GB max
				}
			case "role":
				health.Role = value
			case "redis_version":
				health.KeyDBVersion = value
			case "lag":
				if lag, err := strconv.Atoi(value); err == nil {
					health.ReplicationLag = int32(lag)
				}
			}
		}
	}
	return nil
}

func (c *Checker) parseClusterInfo(clusterInfo string, health *PodHealth) {
	// Parse cluster-specific information
	lines := strings.Split(clusterInfo, "\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				
				if key == "cluster_state" && value != "ok" {
					health.Healthy = false
				}
			}
		}
	}
}
