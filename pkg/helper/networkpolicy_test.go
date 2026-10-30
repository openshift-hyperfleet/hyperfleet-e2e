package helper

import (
	"context"
	"reflect"
	"testing"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const testPolicyName = "redis-ingress-cl-desire-reapply"

func TestDesireStoreClientPolicy(t *testing.T) {
	store := storePolicyFixture()
	// A port distinct from the default, so the copy is actually proven.
	distinctPort := intstr.FromInt32(1234)
	store.Spec.Ingress[0].Ports[0].Port = &distinctPort

	policy := desireStoreClientPolicy(testPolicyName, "run-42",
		map[string]string{"e2e.hyperfleet.io/desire-store-client": "true"}, store)

	if policy.Name != testPolicyName {
		t.Errorf("expected policy name %q, got %q", testPolicyName, policy.Name)
	}
	if policy.Labels[desireStoreClientRunIDLabel] != "run-42" {
		t.Errorf("expected run-id label run-42, got %+v", policy.Labels)
	}
	if !reflect.DeepEqual(policy.Spec.PodSelector, store.Spec.PodSelector) {
		t.Errorf("expected the store's pod selector %v to be reused, got %v",
			store.Spec.PodSelector, policy.Spec.PodSelector)
	}
	if len(policy.Spec.PolicyTypes) != 1 || policy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Errorf("expected policyTypes [Ingress], got %v", policy.Spec.PolicyTypes)
	}
	if len(policy.Spec.Ingress) != 1 {
		t.Fatalf("expected one ingress rule, got %d", len(policy.Spec.Ingress))
	}
	rule := policy.Spec.Ingress[0]
	if len(rule.From) != 1 || rule.From[0].PodSelector.MatchLabels["e2e.hyperfleet.io/desire-store-client"] != "true" {
		t.Errorf("expected the rule to admit the given pod labels, got %+v", rule.From)
	}
	if len(rule.Ports) != 1 || rule.Ports[0].Port.IntValue() != 1234 {
		t.Errorf("expected the store's port 1234 to be copied, got %+v", rule.Ports)
	}
}

func TestDesireStoreClientPolicyDefaultsPortWithoutStoreRules(t *testing.T) {
	// A store policy with no rules at all: no port to copy.
	store := storePolicyFixture()
	store.Spec.Ingress = nil

	policy := desireStoreClientPolicy(testPolicyName, "run-42",
		map[string]string{"e2e.hyperfleet.io/desire-store-client": "true"}, store)

	rule := policy.Spec.Ingress[0]
	if len(rule.Ports) != 1 || rule.Ports[0].Port.IntValue() != desireStorePort {
		t.Errorf("expected the default port %d, got %+v", desireStorePort, rule.Ports)
	}
	if rule.Ports[0].Protocol == nil || *rule.Ports[0].Protocol != corev1.ProtocolTCP {
		t.Errorf("expected protocol TCP, got %+v", rule.Ports[0].Protocol)
	}
}

func TestAdmitDesireStoreClientCreatesAndRestoresPolicy(t *testing.T) {
	h := fakeHelper(storePolicyFixture())
	ctx := context.Background()

	restore, err := h.AdmitDesireStoreClient(ctx, testPolicyName,
		map[string]string{"e2e.hyperfleet.io/desire-store-client": "true"})
	if err != nil {
		t.Fatalf("AdmitDesireStoreClient failed: %v", err)
	}

	policies := h.K8sClient.NetworkingV1().NetworkPolicies("test-ns")
	ours, err := policies.Get(ctx, testPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("the client policy should exist after admit: %v", err)
	}
	if ours.Labels[desireStoreClientRunIDLabel] != "run-42" {
		t.Errorf("expected run-id label run-42, got %+v", ours.Labels)
	}

	if err := restore(ctx); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	if _, err := policies.Get(ctx, testPolicyName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Errorf("the client policy should be gone after restore, got err %v", err)
	}
	if _, err := policies.Get(ctx, desireStoreNetworkPolicyName, metav1.GetOptions{}); err != nil {
		t.Errorf("the store's own policy must survive restore: %v", err)
	}
}

func TestAdmitDesireStoreClientWithoutStorePolicyIsNoOp(t *testing.T) {
	// No store policy means the store is unrestricted, and creating one would
	// isolate it (see AdmitDesireStoreClient).
	h := fakeHelper()
	ctx := context.Background()

	restore, err := h.AdmitDesireStoreClient(ctx, testPolicyName,
		map[string]string{"e2e.hyperfleet.io/desire-store-client": "true"})
	if err != nil {
		t.Fatalf("admit should be a no-op when the store is unrestricted: %v", err)
	}

	list, err := h.K8sClient.NetworkingV1().NetworkPolicies("test-ns").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("failed to list policies: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("no policy should be created when the store is unrestricted, got %+v", list.Items)
	}
	if err := restore(ctx); err != nil {
		t.Fatalf("restore should be a no-op: %v", err)
	}
}

func TestAdmitDesireStoreClientKeepsLeftoverPolicy(t *testing.T) {
	// A leftover from a killed prior run: kept as-is, like EnsureServiceAccount
	// keeps an existing ServiceAccount. The admission content is deterministic,
	// so only the run-id label is stale.
	leftover := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPolicyName,
			Namespace: "test-ns",
			Labels:    map[string]string{desireStoreClientRunIDLabel: "run-old"},
		},
	}
	h := fakeHelper(storePolicyFixture(), leftover)
	ctx := context.Background()

	restore, err := h.AdmitDesireStoreClient(ctx, testPolicyName,
		map[string]string{"e2e.hyperfleet.io/desire-store-client": "true"})
	if err != nil {
		t.Fatalf("a leftover policy from a killed prior run must be kept, not fatal: %v", err)
	}

	ours, err := h.K8sClient.NetworkingV1().NetworkPolicies("test-ns").Get(ctx, testPolicyName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get the kept policy: %v", err)
	}
	if ours.Labels[desireStoreClientRunIDLabel] != "run-old" {
		t.Errorf("the leftover policy must be kept as-is (stale run-id and all), got labels %+v", ours.Labels)
	}

	if err := restore(ctx); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
}

// storePolicyFixture resembles the desire store's own policy: it selects the
// Redis pods and admits the infra-declared clients on the store's port.
func storePolicyFixture() *networkingv1.NetworkPolicy {
	port := intstr.FromInt32(desireStorePort)
	protocol := corev1.ProtocolTCP
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: desireStoreNetworkPolicyName, Namespace: "test-ns"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "redis"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "hyperfleet-applier"},
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{Protocol: &protocol, Port: &port}},
			}},
		},
	}
}

// fakeHelper builds a Helper around a fake clientset, for the policies in
// the namespace the config names.
func fakeHelper(objects ...runtime.Object) *Helper {
	return &Helper{
		Cfg:       &config.Config{Namespace: "test-ns", RunID: "run-42"},
		K8sClient: &k8sclient.Client{Interface: k8sfake.NewClientset(objects...)},
	}
}
