package resources

import (
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	keydbv1alpha1 "github.com/your-org/keydb-operator/api/v1alpha1"
)

// NewPodDisruptionBudget creates a PodDisruptionBudget for the KeyDBCluster
func NewPodDisruptionBudget(keydbCluster *keydbv1alpha1.KeyDBCluster) *policyv1.PodDisruptionBudget {
	labels := getLabels(keydbCluster)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-keydb-pdb", keydbCluster.Name),
			Namespace: keydbCluster.Namespace,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	// Set disruption budget based on configuration
	if keydbCluster.Spec.PodDisruptionBudget.MinAvailable != nil {
		pdb.Spec.MinAvailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: *keydbCluster.Spec.PodDisruptionBudget.MinAvailable,
		}
	} else if keydbCluster.Spec.PodDisruptionBudget.MaxUnavailable != nil {
		pdb.Spec.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: *keydbCluster.Spec.PodDisruptionBudget.MaxUnavailable,
		}
	} else {
		// Default: allow at most 1 pod to be unavailable
		pdb.Spec.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 1,
		}
	}

	return pdb
}
