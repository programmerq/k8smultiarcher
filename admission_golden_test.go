package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// updateGolden regenerates the golden files instead of comparing against them.
// Run: go test -run TestProcessAdmissionReview_Golden -update
var updateGolden = flag.Bool("update", false, "update golden files in testdata/")

const goldenImage = "nginx:latest"

// goldenConfig is a fixed multi-platform config so the rendered response is deterministic.
func goldenConfig() *PlatformTolerationConfig {
	return &PlatformTolerationConfig{
		Mappings: []PlatformTolerationMapping{
			{
				Platform: "linux/arm64",
				Toleration: corev1.Toleration{
					Key:      "arch",
					Value:    "arm64",
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			{
				Platform: "linux/amd64",
				Toleration: corev1.Toleration{
					Key:      "arch",
					Value:    "amd64",
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
		},
	}
}

// canonicalResponse renders an AdmissionResponse into stable, human-readable JSON.
// The JSON patch is decoded and its operations are sorted by their canonical
// serialization, so non-deterministic patch-generation order cannot cause flaky
// golden mismatches. What remains in the golden is the meaningful response shape:
// allowed, patch type, and the set of patch operations.
func canonicalResponse(t *testing.T, resp *admissionv1.AdmissionResponse) []byte {
	t.Helper()

	var patch []map[string]any
	if len(resp.Patch) > 0 {
		if err := json.Unmarshal(resp.Patch, &patch); err != nil {
			t.Fatalf("unmarshal patch: %v", err)
		}
		// Sort by each op's canonical serialization so jsonpatch.CreatePatch's
		// non-deterministic op order can't cause flaky golden mismatches. The op
		// and its key are sorted together; keys are precomputed (with error
		// handling) since a sort comparator can't fail.
		type keyedOp struct {
			op  map[string]any
			key string
		}
		keyed := make([]keyedOp, len(patch))
		for i, op := range patch {
			b, err := json.Marshal(op)
			if err != nil {
				t.Fatalf("marshal patch op: %v", err)
			}
			keyed[i] = keyedOp{op: op, key: string(b)}
		}
		sort.SliceStable(keyed, func(i, j int) bool { return keyed[i].key < keyed[j].key })
		for i := range keyed {
			patch[i] = keyed[i].op
		}
	}

	view := struct {
		UID       string                 `json:"uid"`
		Allowed   bool                   `json:"allowed"`
		PatchType *admissionv1.PatchType `json:"patchType,omitempty"`
		Patch     []map[string]any       `json:"patch,omitempty"`
	}{
		UID:       string(resp.UID),
		Allowed:   resp.Allowed,
		PatchType: resp.PatchType,
		Patch:     patch,
	}

	out, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		t.Fatalf("marshal canonical response: %v", err)
	}
	return append(out, '\n')
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// admissionReviewBytes wraps a raw object into an AdmissionReview request body.
func admissionReviewBytes(t *testing.T, kind metav1.GroupVersionKind, raw []byte) []byte {
	t.Helper()
	review := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Request: &admissionv1.AdmissionRequest{
			UID:    "golden-uid",
			Kind:   kind,
			Object: runtime.RawExtension{Raw: raw},
		},
	}
	return mustMarshal(t, review)
}

func goldenPodBody(t *testing.T) []byte {
	t.Helper()
	pod := &corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "golden-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers:  []corev1.Container{{Name: "nginx", Image: goldenImage}},
			Tolerations: []corev1.Toleration{},
		},
	}
	return admissionReviewBytes(t, metav1.GroupVersionKind{Version: "v1", Kind: "Pod"}, mustMarshal(t, pod))
}

func goldenDaemonSetBody(t *testing.T) []byte {
	t.Helper()
	ds := &appsv1.DaemonSet{
		TypeMeta:   metav1.TypeMeta{Kind: "DaemonSet", APIVersion: "apps/v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "golden-daemonset", Namespace: "default"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "golden"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "golden"}},
				Spec: corev1.PodSpec{
					Containers:  []corev1.Container{{Name: "nginx", Image: goldenImage}},
					Tolerations: []corev1.Toleration{},
				},
			},
		},
	}
	kind := metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"}
	return admissionReviewBytes(t, kind, mustMarshal(t, ds))
}

// TestProcessAdmissionReview_Golden pins the webhook's AdmissionReview response
// shape against checked-in golden files. It guards against k8s JSON
// serialization / defaulting drift that the compiler cannot catch (a renamed or
// removed struct field is a build failure; a changed wire shape is not).
func TestProcessAdmissionReview_Golden(t *testing.T) {
	cache := NewInMemoryCache(cacheSizeDefault)
	cache.Set(goldenImage+":linux/arm64", true, 0)
	cache.Set(goldenImage+":linux/amd64", true, 0)

	cfg := goldenConfig()

	tests := []struct {
		name   string
		golden string
		body   []byte
	}{
		{"pod", "pod_response.json", goldenPodBody(t)},
		{"daemonset", "daemonset_response.json", goldenDaemonSetBody(t)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ProcessAdmissionReview(context.Background(), cache, cfg, nil, tc.body)
			if err != nil {
				t.Fatalf("ProcessAdmissionReview failed: %v", err)
			}
			if result.Response == nil {
				t.Fatal("Response is nil")
			}

			got := canonicalResponse(t, result.Response)
			goldenPath := filepath.Join("testdata", tc.golden)

			if *updateGolden {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden (regenerate with `go test -run TestProcessAdmissionReview_Golden -update`): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("response shape changed vs %s.\n--- got ---\n%s\nIf this change is intended, regenerate with "+
					"`go test -run TestProcessAdmissionReview_Golden -update`.", goldenPath, got)
			}
		})
	}
}
