package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestIsNamespaceDisabled(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		nsToCreate  *corev1.Namespace
		expected    bool
		description string
	}{
		{
			name:        "empty namespace returns false",
			namespace:   "",
			nsToCreate:  nil,
			expected:    false,
			description: "Empty namespace should always return false",
		},
		{
			name:      "namespace with disabled annotation set to true returns true",
			namespace: "test-ns",
			nsToCreate: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
					Annotations: map[string]string{
						AnnotationNamespaceDisabled: "true",
					},
				},
			},
			expected:    true,
			description: "Namespace with disabled=true should return true",
		},
		{
			name:      "namespace with disabled annotation set to false returns false",
			namespace: "test-ns2",
			nsToCreate: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns2",
					Annotations: map[string]string{
						AnnotationNamespaceDisabled: "false",
					},
				},
			},
			expected:    false,
			description: "Namespace with disabled=false should return false",
		},
		{
			name:      "namespace with disabled annotation set to other value returns false",
			namespace: "test-ns3",
			nsToCreate: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns3",
					Annotations: map[string]string{
						AnnotationNamespaceDisabled: "yes",
					},
				},
			},
			expected:    false,
			description: "Namespace with disabled=yes (not 'true') should return false",
		},
		{
			name:      "namespace without annotations returns false",
			namespace: "test-ns4",
			nsToCreate: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ns4",
					Annotations: nil,
				},
			},
			expected:    false,
			description: "Namespace with nil annotations should return false",
		},
		{
			name:      "namespace with empty annotations map returns false",
			namespace: "test-ns5",
			nsToCreate: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-ns5",
					Annotations: map[string]string{},
				},
			},
			expected:    false,
			description: "Namespace with empty annotations map should return false",
		},
		{
			name:        "namespace not found returns false",
			namespace:   "nonexistent-ns",
			nsToCreate:  nil,
			expected:    false,
			description: "Non-existent namespace should return false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the kubeClient singleton for each test
			kubeClientOnce = sync.Once{}

			// Execute the Once.Do to prevent getKubeClient from trying to initialize
			kubeClientOnce.Do(func() {
				// Create a fake clientset with the namespace if provided
				if tt.nsToCreate != nil {
					kubeClient = fake.NewSimpleClientset(tt.nsToCreate)
				} else {
					kubeClient = fake.NewSimpleClientset()
				}
				kubeClientErr = nil
			})

			got := IsNamespaceDisabled(context.Background(), tt.namespace)
			if got != tt.expected {
				t.Errorf("IsNamespaceDisabled() = %v, want %v (%s)", got, tt.expected, tt.description)
			}
		})
	}
}

func TestIsNamespaceDisabled_ClientError(t *testing.T) {
	// Reset the kubeClient singleton
	kubeClientOnce = sync.Once{}

	// Execute the Once.Do to set up an error scenario
	kubeClientOnce.Do(func() {
		kubeClient = nil
		kubeClientErr = errors.New("simulated client error")
	})

	// This should return false when client is unavailable
	got := IsNamespaceDisabled(context.Background(), "test-ns")
	if got != false {
		t.Errorf("IsNamespaceDisabled() with unavailable client = %v, want false", got)
	}
}
