package helper

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// desireStoreNetworkPolicyName is the ingress NetworkPolicy of the desire
	// store's Redis (hyperfleet-infra chart helm/redis, release "redis").
	desireStoreNetworkPolicyName = "redis-ingress"

	// desireStorePort is the Redis Service port, used for the ingress rule
	// when the store's policy carries no rule to copy the port from.
	desireStorePort = 6379

	// desireStoreClientRunIDLabel ties the policies this framework creates to
	// the run, so the suite-wide sweep (CleanupKubeResources) removes one
	// that a spec's own cleanup missed.
	desireStoreClientRunIDLabel = "e2e.hyperfleet.io/run-id"
)

// AdmitDesireStoreClient creates the NetworkPolicy policyName admitting the
// pods matching matchLabels to the desire store's Redis, and returns a
// restore function that deletes the policy again.
func (h *Helper) AdmitDesireStoreClient(
	ctx context.Context, policyName string, matchLabels map[string]string,
) (func(context.Context) error, error) {
	policies := h.K8sClient.NetworkingV1().NetworkPolicies(h.Cfg.Namespace)

	store, err := policies.Get(ctx, desireStoreNetworkPolicyName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		slog.Info("desire store NetworkPolicy not found, nothing to admit",
			"name", desireStoreNetworkPolicyName, "namespace", h.Cfg.Namespace)
		return func(context.Context) error { return nil }, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get NetworkPolicy %s/%s: %w",
			h.Cfg.Namespace, desireStoreNetworkPolicyName, err)
	}

	policy := desireStoreClientPolicy(policyName, h.Cfg.RunID, matchLabels, store)
	policy.Namespace = h.Cfg.Namespace
	if _, err := policies.Create(ctx, policy, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create NetworkPolicy %s/%s: %w",
				h.Cfg.Namespace, policyName, err)
		}
		// A leftover from a killed prior run is kept, like EnsureServiceAccount
		// keeps an existing ServiceAccount: the admission content is
		// deterministic (same name, peer label, store-derived selector), and
		// the restore deletes by name. Only the leftover's run-id label is
		// stale, so the sweep's insurance for that one object — already lost
		// once, when the prior run's cleanup failed — stays lost.
		slog.Info("keeping leftover desire store client NetworkPolicy",
			"namespace", h.Cfg.Namespace, "name", policyName)
	}
	slog.Info("created desire store client NetworkPolicy",
		"namespace", h.Cfg.Namespace, "name", policyName, "match_labels", matchLabels)

	return func(ctx context.Context) error {
		err := policies.Delete(ctx, policyName, metav1.DeleteOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to delete NetworkPolicy %s/%s: %w",
				h.Cfg.Namespace, policyName, err)
		}
		return nil
	}, nil
}

// desireStoreClientPolicy builds the policy that admits the pods matching
// matchLabels to the same pods, and on the same port, the store's own policy
// protects: NetworkPolicies are additive, so it widens access for those pods
// without touching the rules hyperfleet-infra deployed.
func desireStoreClientPolicy(
	policyName, runID string, matchLabels map[string]string, store *networkingv1.NetworkPolicy,
) *networkingv1.NetworkPolicy {
	// The rule's port is copied from the store's first rule when present, so
	// it cannot drift from the store's Service port; otherwise it defaults to
	// TCP on the chart's store port (a store policy with no rules at all).
	var ports []networkingv1.NetworkPolicyPort
	if len(store.Spec.Ingress) > 0 && len(store.Spec.Ingress[0].Ports) > 0 {
		ports = store.Spec.Ingress[0].Ports
	} else {
		protocol := corev1.ProtocolTCP
		ports = []networkingv1.NetworkPolicyPort{{
			Protocol: &protocol,
			Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: desireStorePort},
		}}
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:   policyName,
			Labels: map[string]string{desireStoreClientRunIDLabel: runID},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: store.Spec.PodSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
				}},
				Ports: ports,
			}},
		},
	}
}
