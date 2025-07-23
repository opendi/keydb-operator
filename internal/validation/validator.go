package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	keydbv1alpha1 "github.com/your-org/keydb-operator/api/v1alpha1"
)

// Validator provides validation functionality for KeyDB cluster configurations
type Validator struct{}

// NewValidator creates a new configuration validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateKeyDBCluster validates the entire KeyDBCluster configuration
func (v *Validator) ValidateKeyDBCluster(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if err := v.validateMode(keydbCluster); err != nil {
		return fmt.Errorf("mode validation failed: %w", err)
	}

	if err := v.validateReplicas(keydbCluster); err != nil {
		return fmt.Errorf("replicas validation failed: %w", err)
	}

	if err := v.validateImage(keydbCluster); err != nil {
		return fmt.Errorf("image validation failed: %w", err)
	}

	if err := v.validateConfig(keydbCluster); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if err := v.validateStorage(keydbCluster); err != nil {
		return fmt.Errorf("storage validation failed: %w", err)
	}

	if err := v.validateTLS(keydbCluster); err != nil {
		return fmt.Errorf("TLS validation failed: %w", err)
	}

	if err := v.validateMonitoring(keydbCluster); err != nil {
		return fmt.Errorf("monitoring validation failed: %w", err)
	}

	if err := v.validateUpgrade(keydbCluster); err != nil {
		return fmt.Errorf("upgrade validation failed: %w", err)
	}

	return nil
}

func (v *Validator) validateMode(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	switch keydbCluster.Spec.Mode {
	case keydbv1alpha1.MultiMasterMode:
		return v.validateMultiMasterMode(keydbCluster)
	case keydbv1alpha1.ClusterMode:
		return v.validateClusterMode(keydbCluster)
	default:
		return fmt.Errorf("unsupported mode: %s", keydbCluster.Spec.Mode)
	}
}

func (v *Validator) validateMultiMasterMode(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Replicas < 1 {
		return fmt.Errorf("multi-master mode requires at least 1 replica")
	}

	if keydbCluster.Spec.Replicas > 10 {
		return fmt.Errorf("multi-master mode supports maximum 10 replicas, got %d", keydbCluster.Spec.Replicas)
	}

	// Validate multi-master specific configuration
	if keydbCluster.Spec.MultiMaster != nil {
		// Additional validations can be added here
	}

	return nil
}

func (v *Validator) validateClusterMode(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Cluster == nil {
		return fmt.Errorf("cluster configuration is required for cluster mode")
	}

	if keydbCluster.Spec.Cluster.Shards < 3 {
		return fmt.Errorf("cluster mode requires at least 3 shards, got %d", keydbCluster.Spec.Cluster.Shards)
	}

	if keydbCluster.Spec.Cluster.Shards > 1000 {
		return fmt.Errorf("cluster mode supports maximum 1000 shards, got %d", keydbCluster.Spec.Cluster.Shards)
	}

	if keydbCluster.Spec.Cluster.ReplicasPerShard < 0 {
		return fmt.Errorf("replicas per shard cannot be negative, got %d", keydbCluster.Spec.Cluster.ReplicasPerShard)
	}

	if keydbCluster.Spec.Cluster.ReplicasPerShard > 5 {
		return fmt.Errorf("maximum 5 replicas per shard supported, got %d", keydbCluster.Spec.Cluster.ReplicasPerShard)
	}

	return nil
}

func (v *Validator) validateReplicas(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Replicas < 1 {
		return fmt.Errorf("replicas must be at least 1, got %d", keydbCluster.Spec.Replicas)
	}

	return nil
}

func (v *Validator) validateImage(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Image == "" {
		return fmt.Errorf("image cannot be empty")
	}

	// Basic image format validation
	imageRegex := regexp.MustCompile(`^[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$`)
	if !imageRegex.MatchString(keydbCluster.Spec.Image) {
		return fmt.Errorf("invalid image format: %s", keydbCluster.Spec.Image)
	}

	return nil
}

func (v *Validator) validateConfig(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Config == nil {
		return nil // Config is optional
	}

	config := keydbCluster.Spec.Config

	// Validate memory configuration
	if config.MaxMemory != "" {
		if err := v.validateMemorySize(config.MaxMemory); err != nil {
			return fmt.Errorf("invalid maxMemory: %w", err)
		}
	}

	// Validate password configuration
	if config.RequirePass != nil {
		if config.RequirePass.Value == "" && config.RequirePass.SecretRef == nil {
			return fmt.Errorf("password must be specified either as value or secretRef")
		}
		if config.RequirePass.Value != "" && config.RequirePass.SecretRef != nil {
			return fmt.Errorf("password cannot be specified both as value and secretRef")
		}
		if config.RequirePass.SecretRef != nil {
			if config.RequirePass.SecretRef.Name == "" || config.RequirePass.SecretRef.Key == "" {
				return fmt.Errorf("secretRef must specify both name and key")
			}
		}
	}

	// Validate custom configuration
	if config.CustomConfig != nil {
		for key, value := range config.CustomConfig {
			if err := v.validateCustomConfigParam(key, value); err != nil {
				return fmt.Errorf("invalid custom config parameter %s: %w", key, err)
			}
		}
	}

	return nil
}

func (v *Validator) validateStorage(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Storage == nil {
		return nil // Storage is optional (defaults will be set)
	}

	storage := keydbCluster.Spec.Storage

	// Validate storage size
	if storage.Size != "" {
		if err := v.validateStorageSize(storage.Size); err != nil {
			return fmt.Errorf("invalid storage size: %w", err)
		}
	}

	return nil
}

func (v *Validator) validateTLS(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Config == nil || keydbCluster.Spec.Config.TLS == nil {
		return nil // TLS is optional
	}

	tls := keydbCluster.Spec.Config.TLS

	if tls.Enabled && tls.SecretName == "" {
		return fmt.Errorf("TLS secret name is required when TLS is enabled")
	}

	return nil
}

func (v *Validator) validateMonitoring(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Monitoring == nil {
		return nil // Monitoring is optional
	}

	monitoring := keydbCluster.Spec.Monitoring

	if monitoring.Port != 0 {
		if monitoring.Port < 1024 || monitoring.Port > 65535 {
			return fmt.Errorf("monitoring port must be between 1024 and 65535, got %d", monitoring.Port)
		}
	}

	return nil
}

func (v *Validator) validateUpgrade(keydbCluster *keydbv1alpha1.KeyDBCluster) error {
	if keydbCluster.Spec.Upgrade == nil {
		return nil // Upgrade config is optional
	}

	upgrade := keydbCluster.Spec.Upgrade

	if upgrade.Strategy != "" && upgrade.Strategy != "RollingUpdate" && upgrade.Strategy != "Recreate" {
		return fmt.Errorf("unsupported upgrade strategy: %s", upgrade.Strategy)
	}

	if upgrade.MaxUnavailable < 0 {
		return fmt.Errorf("maxUnavailable cannot be negative, got %d", upgrade.MaxUnavailable)
	}

	if upgrade.ValidationTimeoutSeconds < 0 {
		return fmt.Errorf("validationTimeoutSeconds cannot be negative, got %d", upgrade.ValidationTimeoutSeconds)
	}

	return nil
}

func (v *Validator) validateMemorySize(size string) error {
	// Validate memory size format (e.g., "1gb", "512mb", "2GB")
	memoryRegex := regexp.MustCompile(`^(\d+)(kb|mb|gb|KB|MB|GB)$`)
	if !memoryRegex.MatchString(size) {
		return fmt.Errorf("invalid memory size format: %s (expected format: 1gb, 512mb, etc.)", size)
	}

	return nil
}

func (v *Validator) validateStorageSize(size string) error {
	// Validate Kubernetes resource quantity format
	storageRegex := regexp.MustCompile(`^(\d+)(Gi|Mi|Ki|G|M|K|Ti|Pi|Ei)$`)
	if !storageRegex.MatchString(size) {
		return fmt.Errorf("invalid storage size format: %s (expected format: 10Gi, 1Ti, etc.)", size)
	}

	return nil
}

func (v *Validator) validateCustomConfigParam(key, value string) error {
	// List of dangerous configuration parameters that should not be overridden
	dangerousParams := []string{
		"port", "bind", "daemonize", "pidfile", "logfile",
		"dir", "dbfilename", "cluster-enabled", "cluster-config-file",
		"multi-master", "active-replica",
	}

	for _, dangerous := range dangerousParams {
		if strings.EqualFold(key, dangerous) {
			return fmt.Errorf("parameter %s cannot be overridden via custom config", key)
		}
	}

	// Validate specific parameter formats
	switch strings.ToLower(key) {
	case "timeout":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("timeout must be a number, got %s", value)
		}
	case "tcp-keepalive":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("tcp-keepalive must be a number, got %s", value)
		}
	case "maxclients":
		if clients, err := strconv.Atoi(value); err != nil || clients < 1 {
			return fmt.Errorf("maxclients must be a positive number, got %s", value)
		}
	}

	return nil
}
