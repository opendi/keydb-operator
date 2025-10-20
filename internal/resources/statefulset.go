package resources

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	keydbv1alpha1 "github.com/opendi/keydb-operator/api/v1alpha1"
)

// NewStatefulSet creates a new StatefulSet for the KeyDBCluster
func NewStatefulSet(keydbCluster *keydbv1alpha1.KeyDBCluster) *appsv1.StatefulSet {
	labels := getLabels(keydbCluster)
	replicas := calculateReplicas(keydbCluster)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-keydb", keydbCluster.Name),
			Namespace: keydbCluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: fmt.Sprintf("%s-keydb-headless", keydbCluster.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "keydb-pods",
					Containers: []corev1.Container{
						{
							Name:  "keydb",
							Image: keydbCluster.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "keydb",
									ContainerPort: 6379,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "cluster-bus",
									ContainerPort: 16379,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Command: []string{
								"keydb-server",
								"/etc/keydb/keydb.conf",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/keydb",
								},
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							LivenessProbe:  buildLivenessProbe(keydbCluster),
							ReadinessProbe: buildReadinessProbe(keydbCluster),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-keydb-config", keydbCluster.Name),
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(keydbCluster.Spec.Storage.Size),
							},
						},
					},
				},
			},
		},
	}

	// Set storage class if specified
	if keydbCluster.Spec.Storage.StorageClass != "" {
		statefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName = &keydbCluster.Spec.Storage.StorageClass
	}

	// Set resource requirements if specified
	if keydbCluster.Spec.Resources != nil {
		statefulSet.Spec.Template.Spec.Containers[0].Resources = *keydbCluster.Spec.Resources
	}

	// Add topology spread constraints if enabled
	if keydbCluster.Spec.TopologySpreadConstraints != nil && keydbCluster.Spec.TopologySpreadConstraints.Enabled {
		addTopologySpreadConstraints(statefulSet, keydbCluster)
	}

	// Add TLS support if enabled
	if keydbCluster.Spec.Config != nil && keydbCluster.Spec.Config.TLS != nil && keydbCluster.Spec.Config.TLS.Enabled {
		addTLSSupport(statefulSet, keydbCluster)
	}

	// Add monitoring sidecar if enabled
	if keydbCluster.Spec.Monitoring != nil && keydbCluster.Spec.Monitoring.Enabled {
		addMonitoringSidecar(statefulSet, keydbCluster)
	}

	// Add init container for cluster mode
	if keydbCluster.Spec.Mode == keydbv1alpha1.ClusterMode {
		statefulSet.Spec.Template.Spec.InitContainers = []corev1.Container{
			{
				Name:  "cluster-init",
				Image: keydbCluster.Spec.Image,
				Command: []string{
					"sh",
					"-c",
					getClusterInitScript(keydbCluster),
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "config",
						MountPath: "/etc/keydb",
					},
				},
			},
		}
	}

	// Add post-start hook for multi-master mode
	if keydbCluster.Spec.Mode == keydbv1alpha1.MultiMasterMode {
		statefulSet.Spec.Template.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						getMultiMasterInitScript(keydbCluster),
					},
				},
			},
		}
	}

	return statefulSet
}

func calculateReplicas(keydbCluster *keydbv1alpha1.KeyDBCluster) int32 {
	switch keydbCluster.Spec.Mode {
	case keydbv1alpha1.MultiMasterMode:
		return keydbCluster.Spec.Replicas
	case keydbv1alpha1.ClusterMode:
		if keydbCluster.Spec.Cluster != nil {
			return keydbCluster.Spec.Cluster.Shards * (1 + keydbCluster.Spec.Cluster.ReplicasPerShard)
		}
		return 3 // Default: 3 masters with no replicas
	default:
		return keydbCluster.Spec.Replicas
	}
}

func getClusterInitScript(keydbCluster *keydbv1alpha1.KeyDBCluster) string {
	return fmt.Sprintf(`
# Wait for all pods to be ready
REPLICAS=%d
NAMESPACE=%s
CLUSTER_NAME=%s

echo "Waiting for all pods to be ready..."
for i in $(seq 0 $((REPLICAS-1))); do
  while ! nslookup ${CLUSTER_NAME}-keydb-${i}.${CLUSTER_NAME}-keydb-headless.${NAMESPACE}.svc.cluster.local; do
    echo "Waiting for pod ${i}..."
    sleep 2
  done
done

echo "All pods are ready. Cluster initialization will be handled by the first pod."
`, calculateReplicas(keydbCluster), keydbCluster.Namespace, keydbCluster.Name)
}

func getMultiMasterInitScript(keydbCluster *keydbv1alpha1.KeyDBCluster) string {
	return fmt.Sprintf(`
# Configure multi-master replication
REPLICAS=%d
NAMESPACE=%s
CLUSTER_NAME=%s
POD_NAME=${HOSTNAME}

# Extract pod index from hostname
POD_INDEX=${POD_NAME##*-}

echo "Configuring multi-master replication for pod ${POD_INDEX}..."

# Wait for KeyDB to be ready to accept commands
echo "Waiting for KeyDB to be ready..."
MAX_ATTEMPTS=60
ATTEMPT=0
while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  if keydb-cli ping > /dev/null 2>&1; then
    # Check if still loading from disk
    LOADING=$(keydb-cli info persistence 2>/dev/null | grep "^loading:" | cut -d: -f2 | tr -d '\r')
    if [ "$LOADING" = "0" ]; then
      echo "KeyDB is ready and not loading"
      break
    else
      echo "KeyDB is still loading data from disk..."
    fi
  else
    echo "KeyDB not ready yet (attempt $((ATTEMPT+1))/$MAX_ATTEMPTS)..."
  fi
  ATTEMPT=$((ATTEMPT+1))
  sleep 2
done

if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
  echo "ERROR: KeyDB did not become ready in time"
  exit 1
fi

# Connect to other masters
for i in $(seq 0 $((REPLICAS-1))); do
  if [ "$i" != "$POD_INDEX" ]; then
    echo "Connecting to master ${i}..."
    keydb-cli REPLICAOF ${CLUSTER_NAME}-keydb-${i}.${CLUSTER_NAME}-keydb-headless.${NAMESPACE}.svc.cluster.local 6379 || true
  fi
done

echo "Multi-master configuration completed for pod ${POD_INDEX}"
`, keydbCluster.Spec.Replicas, keydbCluster.Namespace, keydbCluster.Name)
}

func addTLSSupport(statefulSet *appsv1.StatefulSet, keydbCluster *keydbv1alpha1.KeyDBCluster) {
	// Add TLS port
	statefulSet.Spec.Template.Spec.Containers[0].Ports = append(
		statefulSet.Spec.Template.Spec.Containers[0].Ports,
		corev1.ContainerPort{
			Name:          "keydb-tls",
			ContainerPort: 6380,
			Protocol:      corev1.ProtocolTCP,
		},
	)

	// Add TLS volume mount
	statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(
		statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts,
		corev1.VolumeMount{
			Name:      "tls-certs",
			MountPath: "/etc/keydb/tls",
			ReadOnly:  true,
		},
	)

	// Add TLS volume
	statefulSet.Spec.Template.Spec.Volumes = append(
		statefulSet.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name: "tls-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: keydbCluster.Spec.Config.TLS.SecretName,
				},
			},
		},
	)
}

func addMonitoringSidecar(statefulSet *appsv1.StatefulSet, keydbCluster *keydbv1alpha1.KeyDBCluster) {
	port := int32(9121)
	if keydbCluster.Spec.Monitoring.Port != 0 {
		port = keydbCluster.Spec.Monitoring.Port
	}

	// Add Redis exporter sidecar
	exporterContainer := corev1.Container{
		Name:  "redis-exporter",
		Image: "oliver006/redis_exporter:latest",
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: []corev1.EnvVar{
			{
				Name:  "REDIS_ADDR",
				Value: "redis://localhost:6379",
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	}

	statefulSet.Spec.Template.Spec.Containers = append(
		statefulSet.Spec.Template.Spec.Containers,
		exporterContainer,
	)
}

func addTopologySpreadConstraints(statefulSet *appsv1.StatefulSet, keydbCluster *keydbv1alpha1.KeyDBCluster) {
	config := keydbCluster.Spec.TopologySpreadConstraints
	labels := getLabels(keydbCluster)

	// Set defaults
	maxSkew := int32(1)
	if config.MaxSkew > 0 {
		maxSkew = config.MaxSkew
	}

	topologyKey := "topology.kubernetes.io/zone"
	if config.TopologyKey != "" {
		topologyKey = config.TopologyKey
	}

	whenUnsatisfiable := corev1.DoNotSchedule
	if config.WhenUnsatisfiable == "ScheduleAnyway" {
		whenUnsatisfiable = corev1.ScheduleAnyway
	}

	constraint := corev1.TopologySpreadConstraint{
		MaxSkew:           maxSkew,
		TopologyKey:       topologyKey,
		WhenUnsatisfiable: whenUnsatisfiable,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
	}

	statefulSet.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{constraint}
}

func getLabels(keydbCluster *keydbv1alpha1.KeyDBCluster) map[string]string {
	return map[string]string{
		"app":                          "keydb",
		"keydb.io/cluster":             keydbCluster.Name,
		"keydb.io/mode":                string(keydbCluster.Spec.Mode),
		"app.kubernetes.io/name":       "keydb",
		"app.kubernetes.io/instance":   keydbCluster.Name,
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/part-of":    "keydb-cluster",
		"app.kubernetes.io/managed-by": "keydb-operator",
	}
}

// buildLivenessProbe creates a liveness probe based on the cluster configuration
func buildLivenessProbe(keydbCluster *keydbv1alpha1.KeyDBCluster) *corev1.Probe {
	probe := keydbCluster.Spec.LivenessProbe

	// If probe is explicitly disabled, return nil
	if probe != nil && probe.Enabled != nil && !*probe.Enabled {
		return nil
	}

	// Build the probe with defaults or custom values
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: int32OrDefault(probe.GetInitialDelaySeconds(), 5),
		PeriodSeconds:       int32OrDefault(probe.GetPeriodSeconds(), 10),
		TimeoutSeconds:      int32OrDefault(probe.GetTimeoutSeconds(), 1),
		SuccessThreshold:    int32OrDefault(probe.GetSuccessThreshold(), 1),
		FailureThreshold:    int32OrDefault(probe.GetFailureThreshold(), 3),
	}

	// Use custom exec command if provided, otherwise default to keydb-cli PING
	if probe != nil && len(probe.Exec) > 0 {
		livenessProbe.ProbeHandler = corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: probe.Exec,
			},
		}
	} else {
		livenessProbe.ProbeHandler = corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"keydb-cli", "-p", "6379", "PING"},
			},
		}
	}

	return livenessProbe
}

// buildReadinessProbe creates a readiness probe based on the cluster configuration
func buildReadinessProbe(keydbCluster *keydbv1alpha1.KeyDBCluster) *corev1.Probe {
	probe := keydbCluster.Spec.ReadinessProbe

	// If probe is explicitly disabled, return nil
	if probe != nil && probe.Enabled != nil && !*probe.Enabled {
		return nil
	}

	// Build the probe with defaults or custom values
	readinessProbe := &corev1.Probe{
		InitialDelaySeconds: int32OrDefault(probe.GetInitialDelaySeconds(), 5),
		PeriodSeconds:       int32OrDefault(probe.GetPeriodSeconds(), 10),
		TimeoutSeconds:      int32OrDefault(probe.GetTimeoutSeconds(), 5), // Increased timeout for complex checks
		SuccessThreshold:    int32OrDefault(probe.GetSuccessThreshold(), 1),
		FailureThreshold:    int32OrDefault(probe.GetFailureThreshold(), 3),
	}

	// Use custom exec command if provided, otherwise use mode-specific readiness check
	if probe != nil && len(probe.Exec) > 0 {
		readinessProbe.ProbeHandler = corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: probe.Exec,
			},
		}
	} else {
		readinessProbe.ProbeHandler = corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: buildReadinessCommand(keydbCluster),
			},
		}
	}

	return readinessProbe
}

// buildReadinessCommand creates a mode-specific readiness check command
func buildReadinessCommand(keydbCluster *keydbv1alpha1.KeyDBCluster) []string {
	switch keydbCluster.Spec.Mode {
	case keydbv1alpha1.MultiMasterMode:
		return []string{
			"sh",
			"-c",
			`
# Multi-master readiness check
# 1. Check if KeyDB responds to PING
if ! keydb-cli ping > /dev/null 2>&1; then
  echo "KeyDB not responding to PING"
  exit 1
fi

# 2. Check if KeyDB is not loading data from disk
LOADING=$(keydb-cli info persistence 2>/dev/null | grep "^loading:" | cut -d: -f2 | tr -d '\r')
if [ "$LOADING" != "0" ]; then
  echo "KeyDB is still loading data from disk"
  exit 1
fi

# 3. Check if multi-master replication is configured (role should be active-replica)
ROLE=$(keydb-cli info replication 2>/dev/null | grep "^role:" | cut -d: -f2 | tr -d '\r')
if [ "$ROLE" != "active-replica" ]; then
  echo "Multi-master replication not configured yet (role: $ROLE)"
  exit 1
fi

echo "KeyDB is ready for multi-master mode"
exit 0
`,
		}
	case keydbv1alpha1.ClusterMode:
		return []string{
			"sh",
			"-c",
			`
# Cluster mode readiness check
# 1. Check if KeyDB responds to PING
if ! keydb-cli ping > /dev/null 2>&1; then
  echo "KeyDB not responding to PING"
  exit 1
fi

# 2. Check if KeyDB is not loading data from disk
LOADING=$(keydb-cli info persistence 2>/dev/null | grep "^loading:" | cut -d: -f2 | tr -d '\r')
if [ "$LOADING" != "0" ]; then
  echo "KeyDB is still loading data from disk"
  exit 1
fi

echo "KeyDB is ready for cluster mode"
exit 0
`,
		}
	default:
		// Default mode (standalone) - just check PING and loading status
		return []string{
			"sh",
			"-c",
			`
# Standalone mode readiness check
# 1. Check if KeyDB responds to PING
if ! keydb-cli ping > /dev/null 2>&1; then
  echo "KeyDB not responding to PING"
  exit 1
fi

# 2. Check if KeyDB is not loading data from disk
LOADING=$(keydb-cli info persistence 2>/dev/null | grep "^loading:" | cut -d: -f2 | tr -d '\r')
if [ "$LOADING" != "0" ]; then
  echo "KeyDB is still loading data from disk"
  exit 1
fi

echo "KeyDB is ready for standalone mode"
exit 0
`,
		}
	}
}

// int32OrDefault returns the value if not nil, otherwise returns the default
func int32OrDefault(value *int32, defaultValue int32) int32 {
	if value != nil {
		return *value
	}
	return defaultValue
}
