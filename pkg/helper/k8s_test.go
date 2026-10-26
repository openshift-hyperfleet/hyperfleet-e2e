package helper

import (
	"context"
	"strings"
	"testing"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestVerifyNamespaceActive(t *testing.T) {
	tests := []struct {
		namespace           *corev1.Namespace
		expectedLabels      map[string]string
		expectedAnnotations map[string]string
		name                string
		namespaceName       string
		errContains         string
		wantErr             bool
	}{
		{
			name:          "active namespace with matching labels and annotations",
			namespaceName: "test-ns",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"app":  "test",
						"tier": "frontend",
					},
					Annotations: map[string]string{
						"version": "v1",
					},
				},
				Status: corev1.NamespaceStatus{
					Phase: corev1.NamespaceActive,
				},
			},
			expectedLabels: map[string]string{
				"app": "test",
			},
			expectedAnnotations: map[string]string{
				"version": "v1",
			},
			wantErr: false,
		},
		{
			name:          "namespace not found",
			namespaceName: "missing-ns",
			namespace:     nil,
			wantErr:       true,
			errContains:   "not found",
		},
		{
			name:          "namespace not active",
			namespaceName: "terminating-ns",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "terminating-ns",
				},
				Status: corev1.NamespaceStatus{
					Phase: corev1.NamespaceTerminating,
				},
			},
			wantErr:     true,
			errContains: "phase is Terminating",
		},
		{
			name:          "missing expected label",
			namespaceName: "test-ns",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"app": "test",
					},
				},
				Status: corev1.NamespaceStatus{
					Phase: corev1.NamespaceActive,
				},
			},
			expectedLabels: map[string]string{
				"missing": "value",
			},
			wantErr:     true,
			errContains: "missing labels",
		},
		{
			name:          "mismatched label value",
			namespaceName: "test-ns",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Labels: map[string]string{
						"app": "test",
					},
				},
				Status: corev1.NamespaceStatus{
					Phase: corev1.NamespaceActive,
				},
			},
			expectedLabels: map[string]string{
				"app": "wrong",
			},
			wantErr:     true,
			errContains: "mismatched labels",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset()
			if tt.namespace != nil {
				_, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(), tt.namespace, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create test namespace: %v", err)
				}
			}

			h := &Helper{K8sClient: &k8sclient.Client{Interface: fakeClient}}
			err := h.VerifyNamespaceActive(context.TODO(), tt.namespaceName, tt.expectedLabels, tt.expectedAnnotations)

			if tt.wantErr {
				if err == nil {
					t.Errorf("VerifyNamespaceActive() expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("VerifyNamespaceActive() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("VerifyNamespaceActive() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestVerifyJobComplete(t *testing.T) {
	tests := []struct {
		job                 *batchv1.Job
		expectedLabels      map[string]string
		expectedAnnotations map[string]string
		name                string
		namespace           string
		errContains         string
		wantErr             bool
	}{
		{
			name:      "completed job with matching labels",
			namespace: "test-ns",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app":  "test",
						"type": "batch",
					},
					Annotations: map[string]string{
						"generation": "1",
					},
				},
				Status: batchv1.JobStatus{
					Succeeded: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "test",
			},
			expectedAnnotations: map[string]string{
				"generation": "1",
			},
			wantErr: false,
		},
		{
			name:      "job not completed",
			namespace: "test-ns",
			job: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "running-job",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "test",
					},
				},
				Status: batchv1.JobStatus{
					Active: 1,
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "test",
			},
			wantErr:     true,
			errContains: "has not completed successfully",
		},
		{
			name:      "no job found",
			namespace: "test-ns",
			job:       nil,
			expectedLabels: map[string]string{
				"app": "missing",
			},
			wantErr:     true,
			errContains: "no job found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset()

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.namespace,
				},
			}
			_, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Failed to create test namespace: %v", err)
			}

			// Create job if provided
			if tt.job != nil {
				_, createErr := fakeClient.BatchV1().
					Jobs(tt.namespace).
					Create(context.TODO(), tt.job, metav1.CreateOptions{})
				if createErr != nil {
					t.Fatalf("Failed to create test job: %v", createErr)
				}
			}

			h := &Helper{K8sClient: &k8sclient.Client{Interface: fakeClient}}
			err = h.VerifyJobComplete(context.TODO(), tt.namespace, tt.expectedLabels, tt.expectedAnnotations)

			if tt.wantErr {
				if err == nil {
					t.Errorf("VerifyJobComplete() expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("VerifyJobComplete() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("VerifyJobComplete() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestVerifyDeploymentAvailable(t *testing.T) {
	tests := []struct {
		deployment          *appsv1.Deployment
		expectedLabels      map[string]string
		expectedAnnotations map[string]string
		name                string
		namespace           string
		errContains         string
		wantErr             bool
	}{
		{
			name:      "available deployment with matching labels",
			namespace: "test-ns",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deploy",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app":  "web",
						"tier": "frontend",
					},
					Annotations: map[string]string{
						"version": "v1",
					},
				},
				Status: appsv1.DeploymentStatus{
					AvailableReplicas: 3,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentAvailable,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "web",
			},
			expectedAnnotations: map[string]string{
				"version": "v1",
			},
			wantErr: false,
		},
		{
			name:      "deployment not available",
			namespace: "test-ns",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unavailable-deploy",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "web",
					},
				},
				Status: appsv1.DeploymentStatus{
					AvailableReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentAvailable,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "web",
			},
			wantErr:     true,
			errContains: "is not available",
		},
		{
			name:       "no deployment found",
			namespace:  "test-ns",
			deployment: nil,
			expectedLabels: map[string]string{
				"app": "missing",
			},
			wantErr:     true,
			errContains: "no deployment found",
		},
		{
			name:      "deployment with zero replicas but available",
			namespace: "test-ns",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zero-replica-deploy",
					Namespace: "test-ns",
					Labels: map[string]string{
						"app": "web",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(0),
				},
				Status: appsv1.DeploymentStatus{
					AvailableReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:   appsv1.DeploymentAvailable,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"app": "web",
			},
			wantErr: false, // Should be valid with 0 replicas if condition is True
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset()

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: tt.namespace,
				},
			}
			_, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Failed to create test namespace: %v", err)
			}

			// Create deployment if provided
			if tt.deployment != nil {
				_, createErr := fakeClient.AppsV1().
					Deployments(tt.namespace).
					Create(context.TODO(), tt.deployment, metav1.CreateOptions{})
				if createErr != nil {
					t.Fatalf("Failed to create test deployment: %v", createErr)
				}
			}

			h := &Helper{K8sClient: &k8sclient.Client{Interface: fakeClient}}
			err = h.VerifyDeploymentAvailable(context.TODO(), tt.namespace, tt.expectedLabels, tt.expectedAnnotations)

			if tt.wantErr {
				if err == nil {
					t.Errorf("VerifyDeploymentAvailable() expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("VerifyDeploymentAvailable() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("VerifyDeploymentAvailable() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestVerifyMapContains(t *testing.T) {
	tests := []struct {
		actual   map[string]string
		expected map[string]string
		name     string
		mapType  string
		errMsg   string
		wantErr  bool
	}{
		{
			name:     "all expected keys present with matching values",
			actual:   map[string]string{"a": "1", "b": "2", "c": "3"},
			expected: map[string]string{"a": "1", "b": "2"},
			mapType:  "label",
			wantErr:  false,
		},
		{
			name:     "empty expected map",
			actual:   map[string]string{"a": "1"},
			expected: map[string]string{},
			mapType:  "label",
			wantErr:  false,
		},
		{
			name:     "missing key",
			actual:   map[string]string{"a": "1"},
			expected: map[string]string{"b": "2"},
			mapType:  "label",
			wantErr:  true,
			errMsg:   "missing labels: b",
		},
		{
			name:     "mismatched value",
			actual:   map[string]string{"a": "1"},
			expected: map[string]string{"a": "2"},
			mapType:  "label",
			wantErr:  true,
			errMsg:   "mismatched labels: a (expected: 2, actual: 1)",
		},
		{
			name:     "multiple missing and mismatched",
			actual:   map[string]string{"a": "1", "b": "wrong"},
			expected: map[string]string{"a": "1", "b": "2", "c": "3"},
			mapType:  "annotation",
			wantErr:  true,
			errMsg:   "missing annotations: c; mismatched annotations: b (expected: 2, actual: wrong)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyMapContains(tt.actual, tt.expected, tt.mapType)
			if tt.wantErr {
				if err == nil {
					t.Errorf("verifyMapContains() expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("verifyMapContains() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("verifyMapContains() unexpected error = %v", err)
				}
			}
		})
	}
}

// Helper functions
func int32Ptr(i int32) *int32 {
	return &i
}
