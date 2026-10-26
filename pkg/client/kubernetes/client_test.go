package kubernetes

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDeleteNamespaceContinuesAfterCollectionFailures(t *testing.T) {
	t.Parallel()
	fakeClient := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}})
	fakeClient.PrependReactor("delete-collection", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("collection deletion failed")
	})
	c := &Client{Interface: fakeClient}
	if err := c.DeleteNamespaceAndWait(t.Context(), "test-ns"); err != nil {
		t.Fatalf("collection failures prevented namespace cleanup: %v", err)
	}
	collections, namespaceDeletes := 0, 0
	for _, action := range fakeClient.Actions() {
		if action.GetVerb() == "delete-collection" {
			collections++
		}
		if action.GetVerb() == "delete" && action.GetResource().Resource == "namespaces" {
			namespaceDeletes++
		}
	}
	if collections != 4 || namespaceDeletes != 1 {
		t.Fatalf("got %d collection deletes and %d namespace deletes, want 4 and 1", collections, namespaceDeletes)
	}
}
