package resources

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	keydbv1alpha1 "github.com/opendi/keydb-operator/api/v1alpha1"
)

// NewHeadlessService creates a headless service for the KeyDBCluster StatefulSet
func NewHeadlessService(keydbCluster *keydbv1alpha1.KeyDBCluster) *corev1.Service {
	labels := getLabels(keydbCluster)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-keydb-headless", keydbCluster.Name),
			Namespace: keydbCluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None", // Headless service
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "keydb",
					Port:       6379,
					TargetPort: intstr.FromString("keydb"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "cluster-bus",
					Port:       16379,
					TargetPort: intstr.FromString("cluster-bus"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			PublishNotReadyAddresses: true, // Important for StatefulSet discovery
		},
	}
}

// NewClientService creates a client service for external access to the KeyDBCluster
func NewClientService(keydbCluster *keydbv1alpha1.KeyDBCluster) *corev1.Service {
	labels := getLabels(keydbCluster)
	serviceType := corev1.ServiceTypeClusterIP
	port := int32(6379)

	// Use spec values if provided
	if keydbCluster.Spec.Service != nil {
		if keydbCluster.Spec.Service.Type != "" {
			serviceType = corev1.ServiceType(keydbCluster.Spec.Service.Type)
		}
		if keydbCluster.Spec.Service.Port != 0 {
			port = keydbCluster.Spec.Service.Port
		}
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-keydb", keydbCluster.Name),
			Namespace: keydbCluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "keydb",
					Port:       port,
					TargetPort: intstr.FromString("keydb"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Add TLS port if enabled
	if keydbCluster.Spec.Config != nil && keydbCluster.Spec.Config.TLS != nil && keydbCluster.Spec.Config.TLS.Enabled {
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Name:       "keydb-tls",
			Port:       6380,
			TargetPort: intstr.FromString("keydb-tls"),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Add metrics port if monitoring is enabled
	if keydbCluster.Spec.Monitoring != nil && keydbCluster.Spec.Monitoring.Enabled {
		metricsPort := int32(9121)
		if keydbCluster.Spec.Monitoring.Port != 0 {
			metricsPort = keydbCluster.Spec.Monitoring.Port
		}
		service.Spec.Ports = append(service.Spec.Ports, corev1.ServicePort{
			Name:       "metrics",
			Port:       metricsPort,
			TargetPort: intstr.FromString("metrics"),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// Add custom annotations
	if keydbCluster.Spec.Service != nil && keydbCluster.Spec.Service.Annotations != nil {
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		for key, value := range keydbCluster.Spec.Service.Annotations {
			service.Annotations[key] = value
		}
	}

	// For cluster mode, we want to load balance across all nodes
	// For multi-master mode, we can also load balance since all nodes accept writes
	if keydbCluster.Spec.Mode == keydbv1alpha1.ClusterMode {
		// In cluster mode, we might want to add annotations for smart load balancing
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		service.Annotations["keydb.io/cluster-aware"] = "true"
	}

	return service
}

// NewServiceMonitor creates a ServiceMonitor for Prometheus monitoring
func NewServiceMonitor(keydbCluster *keydbv1alpha1.KeyDBCluster) *ServiceMonitor {
	if keydbCluster.Spec.Monitoring == nil || !keydbCluster.Spec.Monitoring.Enabled {
		return nil
	}

	labels := getLabels(keydbCluster)

	// Add custom ServiceMonitor labels if specified
	if keydbCluster.Spec.Monitoring.ServiceMonitorLabels != nil {
		for key, value := range keydbCluster.Spec.Monitoring.ServiceMonitorLabels {
			labels[key] = value
		}
	}

	return &ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-keydb-metrics", keydbCluster.Name),
			Namespace: keydbCluster.Namespace,
			Labels:    labels,
		},
		Spec: ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: getLabels(keydbCluster),
			},
			Endpoints: []Endpoint{
				{
					Port:     "metrics",
					Interval: "30s",
					Path:     "/metrics",
				},
			},
		},
	}
}

// ServiceMonitor represents a Prometheus ServiceMonitor
// This is a simplified version - in practice you'd import from prometheus-operator
type ServiceMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ServiceMonitorSpec `json:"spec"`
}

type ServiceMonitorSpec struct {
	Selector  metav1.LabelSelector `json:"selector"`
	Endpoints []Endpoint           `json:"endpoints"`
}

type Endpoint struct {
	Port     string `json:"port"`
	Interval string `json:"interval"`
	Path     string `json:"path"`
}
