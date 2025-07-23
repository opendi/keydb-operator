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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeyDBClusterMode defines the deployment mode for KeyDB
// +kubebuilder:validation:Enum=multi-master;cluster
type KeyDBClusterMode string

const (
	// MultiMasterMode deploys KeyDB in multi-master configuration
	MultiMasterMode KeyDBClusterMode = "multi-master"
	// ClusterMode deploys KeyDB in cluster mode with sharding
	ClusterMode KeyDBClusterMode = "cluster"
)

// KeyDBClusterPhase defines the current phase of the KeyDB cluster
type KeyDBClusterPhase string

const (
	// PendingPhase indicates the cluster is being created
	PendingPhase KeyDBClusterPhase = "Pending"
	// RunningPhase indicates the cluster is running normally
	RunningPhase KeyDBClusterPhase = "Running"
	// FailedPhase indicates the cluster has failed
	FailedPhase KeyDBClusterPhase = "Failed"
)

// SecretRef represents a reference to a secret
type SecretRef struct {
	// Name of the secret
	Name string `json:"name"`
	// Key in the secret
	Key string `json:"key"`
}

// PasswordConfig defines password authentication configuration
type PasswordConfig struct {
	// Plain text password (not recommended for production)
	// +optional
	Value string `json:"value,omitempty"`
	// Reference to a secret containing the password
	// +optional
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// TLSConfig defines TLS configuration
type TLSConfig struct {
	// Enable TLS
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Secret containing TLS certificates
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// Require client certificates
	// +optional
	RequireClientCerts bool `json:"requireClientCerts,omitempty"`
}

// MonitoringConfig defines monitoring configuration
type MonitoringConfig struct {
	// Enable Prometheus metrics
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Metrics port
	// +kubebuilder:default=9121
	// +optional
	Port int32 `json:"port,omitempty"`
	// ServiceMonitor labels for Prometheus operator
	// +optional
	ServiceMonitorLabels map[string]string `json:"serviceMonitorLabels,omitempty"`
}

// UpgradeConfig defines upgrade strategy configuration
type UpgradeConfig struct {
	// Rolling upgrade strategy
	// +kubebuilder:default="RollingUpdate"
	// +optional
	Strategy string `json:"strategy,omitempty"`
	// Maximum unavailable pods during upgrade
	// +kubebuilder:default=1
	// +optional
	MaxUnavailable int32 `json:"maxUnavailable,omitempty"`
	// Validation timeout for each pod during upgrade
	// +kubebuilder:default=300
	// +optional
	ValidationTimeoutSeconds int32 `json:"validationTimeoutSeconds,omitempty"`
}

// KeyDBConfig defines KeyDB configuration options
type KeyDBConfig struct {
	// Maximum memory usage (e.g., "1gb", "512mb")
	// +optional
	MaxMemory string `json:"maxMemory,omitempty"`
	// Enable persistence
	// +kubebuilder:default=true
	// +optional
	Persistence bool `json:"persistence,omitempty"`
	// Password authentication configuration
	// +optional
	RequirePass *PasswordConfig `json:"requirePass,omitempty"`
	// TLS configuration
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
	// Custom KeyDB configuration parameters
	// +optional
	CustomConfig map[string]string `json:"customConfig,omitempty"`
}

// MultiMasterConfig defines multi-master specific configuration
type MultiMasterConfig struct {
	// Enable active replica mode
	// +kubebuilder:default=true
	// +optional
	ActiveReplica bool `json:"activeReplica,omitempty"`
}

// ClusterConfig defines cluster mode specific configuration
type ClusterConfig struct {
	// Number of shards (masters)
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +optional
	Shards int32 `json:"shards,omitempty"`
	// Number of replicas per shard
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	ReplicasPerShard int32 `json:"replicasPerShard,omitempty"`
}

// StorageConfig defines storage configuration
type StorageConfig struct {
	// Storage size
	// +kubebuilder:default="10Gi"
	// +optional
	Size string `json:"size,omitempty"`
	// Storage class name
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
}

// ServiceConfig defines service configuration
type ServiceConfig struct {
	// Service type
	// +kubebuilder:default="ClusterIP"
	// +optional
	Type string `json:"type,omitempty"`
	// Service port
	// +kubebuilder:default=6379
	// +optional
	Port int32 `json:"port,omitempty"`
	// Service annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PodDisruptionBudgetConfig defines pod disruption budget configuration
type PodDisruptionBudgetConfig struct {
	// Minimum available pods
	// +optional
	MinAvailable *int32 `json:"minAvailable,omitempty"`
	// Maximum unavailable pods
	// +optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}

// TopologySpreadConstraintConfig defines topology spread constraint configuration
type TopologySpreadConstraintConfig struct {
	// Enable topology spread constraints
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Maximum skew allowed between zones
	// +kubebuilder:default=1
	// +optional
	MaxSkew int32 `json:"maxSkew,omitempty"`
	// Topology key to spread across (e.g., "topology.kubernetes.io/zone")
	// +kubebuilder:default="topology.kubernetes.io/zone"
	// +optional
	TopologyKey string `json:"topologyKey,omitempty"`
	// What to do when constraint cannot be satisfied
	// +kubebuilder:default="DoNotSchedule"
	// +kubebuilder:validation:Enum=DoNotSchedule;ScheduleAnyway
	// +optional
	WhenUnsatisfiable string `json:"whenUnsatisfiable,omitempty"`
}

// ProbeConfig defines configuration for liveness and readiness probes
type ProbeConfig struct {
	// Enable the probe (default: true)
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// Command to execute for exec probe
	// +optional
	Exec []string `json:"exec,omitempty"`
	// Initial delay before starting probes
	// +kubebuilder:default=5
	// +optional
	InitialDelaySeconds *int32 `json:"initialDelaySeconds,omitempty"`
	// How often to perform the probe
	// +kubebuilder:default=10
	// +optional
	PeriodSeconds *int32 `json:"periodSeconds,omitempty"`
	// Timeout for the probe
	// +kubebuilder:default=1
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
	// Minimum consecutive successes for the probe to be considered successful
	// +kubebuilder:default=1
	// +optional
	SuccessThreshold *int32 `json:"successThreshold,omitempty"`
	// Minimum consecutive failures for the probe to be considered failed
	// +kubebuilder:default=3
	// +optional
	FailureThreshold *int32 `json:"failureThreshold,omitempty"`
}

// Getter methods for ProbeConfig to handle nil cases
func (p *ProbeConfig) GetInitialDelaySeconds() *int32 {
	if p == nil {
		return nil
	}
	return p.InitialDelaySeconds
}

func (p *ProbeConfig) GetPeriodSeconds() *int32 {
	if p == nil {
		return nil
	}
	return p.PeriodSeconds
}

func (p *ProbeConfig) GetTimeoutSeconds() *int32 {
	if p == nil {
		return nil
	}
	return p.TimeoutSeconds
}

func (p *ProbeConfig) GetSuccessThreshold() *int32 {
	if p == nil {
		return nil
	}
	return p.SuccessThreshold
}

func (p *ProbeConfig) GetFailureThreshold() *int32 {
	if p == nil {
		return nil
	}
	return p.FailureThreshold
}

// KeyDBClusterSpec defines the desired state of KeyDBCluster
type KeyDBClusterSpec struct {
	// Deployment mode
	Mode KeyDBClusterMode `json:"mode"`

	// Number of KeyDB instances (for multi-master mode)
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// KeyDB container image
	// +kubebuilder:default="eqalpha/keydb:latest"
	// +optional
	Image string `json:"image,omitempty"`

	// Multi-master specific configuration
	// +optional
	MultiMaster *MultiMasterConfig `json:"multiMaster,omitempty"`

	// Cluster mode specific configuration
	// +optional
	Cluster *ClusterConfig `json:"cluster,omitempty"`

	// KeyDB configuration options
	// +optional
	Config *KeyDBConfig `json:"config,omitempty"`

	// Resource requirements
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration
	// +optional
	Storage *StorageConfig `json:"storage,omitempty"`

	// Service configuration
	// +optional
	Service *ServiceConfig `json:"service,omitempty"`

	// Monitoring configuration
	// +optional
	Monitoring *MonitoringConfig `json:"monitoring,omitempty"`

	// Upgrade configuration
	// +optional
	Upgrade *UpgradeConfig `json:"upgrade,omitempty"`

	// Pod disruption budget configuration
	// +optional
	PodDisruptionBudget *PodDisruptionBudgetConfig `json:"podDisruptionBudget,omitempty"`

	// Topology spread constraints configuration
	// +optional
	TopologySpreadConstraints *TopologySpreadConstraintConfig `json:"topologySpreadConstraints,omitempty"`

	// Liveness probe configuration
	// +optional
	LivenessProbe *ProbeConfig `json:"livenessProbe,omitempty"`

	// Readiness probe configuration
	// +optional
	ReadinessProbe *ProbeConfig `json:"readinessProbe,omitempty"`
}

// NodeStatus represents the status of an individual KeyDB node
type NodeStatus struct {
	// Node name
	// +optional
	Name string `json:"name,omitempty"`
	// Node role (master/replica)
	// +optional
	Role string `json:"role,omitempty"`
	// Node status
	// +optional
	Status string `json:"status,omitempty"`
	// Node endpoint
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// KeyDBClusterStatus defines the observed state of KeyDBCluster
type KeyDBClusterStatus struct {
	// Current phase
	// +optional
	Phase KeyDBClusterPhase `json:"phase,omitempty"`

	// Total number of replicas
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Number of ready replicas
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Individual node status
	// +optional
	Nodes []NodeStatus `json:"nodes,omitempty"`

	// Current image version being used
	// +optional
	CurrentImage string `json:"currentImage,omitempty"`

	// Upgrade status
	// +optional
	UpgradeStatus *UpgradeStatus `json:"upgradeStatus,omitempty"`
}

// UpgradeStatus represents the status of an ongoing upgrade
type UpgradeStatus struct {
	// Target image for upgrade
	// +optional
	TargetImage string `json:"targetImage,omitempty"`
	// Current upgrade phase
	// +optional
	Phase string `json:"phase,omitempty"`
	// Pods upgraded so far
	// +optional
	UpgradedPods int32 `json:"upgradedPods,omitempty"`
	// Total pods to upgrade
	// +optional
	TotalPods int32 `json:"totalPods,omitempty"`
	// Upgrade start time
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
}

// Health monitoring is now handled by Kubernetes-native liveness/readiness probes
// No custom health metrics are tracked in the operator status

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode"
//+kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
//+kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// KeyDBCluster is the Schema for the keydbclusters API
type KeyDBCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KeyDBClusterSpec   `json:"spec,omitempty"`
	Status KeyDBClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// KeyDBClusterList contains a list of KeyDBCluster
type KeyDBClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KeyDBCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&KeyDBCluster{}, &KeyDBClusterList{})
}
