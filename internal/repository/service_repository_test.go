package repository

import (
	"context"
	"testing"

	"github.com/okdp/okdp-control-plane-server/internal/repository/crd"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestServiceRepositoryUpdateKeepsUnknownFields(t *testing.T) {
	existing := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": crd.ReleaseAPIVersion,
		"kind":       crd.ReleaseKind,
		"metadata": map[string]any{
			"name":      "p1-airflow",
			"namespace": "p1",
			"labels":    map[string]any{crd.LabelProject: "p1"},
		},
		"spec": map[string]any{
			"package": map[string]any{
				"repository": "ghcr.io/okdp/airflow",
				"tag":        "1.0.0",
				"interval":   "30m",
				"secretRef":  map[string]any{"name": "pull"},
			},
			"parameters":      map[string]any{"replicas": int64(1), "image": "a"},
			"targetNamespace": "p1",
			"createNamespace": false,
			"protected":       true,
			"roles":           []any{"airflow"},
			"extensions": map[string]any{
				"okdp.io": map[string]any{"databases": map[string]any{"metadata": map[string]any{"server": "pg"}}},
			},
		},
	}}

	gvr := crd.GetReleaseGVR()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ReleaseList"},
		existing,
	)
	repo := NewServiceRepository(client)
	ctx := context.Background()

	release, err := repo.Get(ctx, "p1", "p1-airflow")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	release.Spec.Package.Tag = "1.1.0"
	release.Spec.Parameters["replicas"] = int64(2)
	if err := repo.Update(ctx, "p1", release); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := client.Resource(gvr).Namespace("p1").Get(ctx, "p1-airflow", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if v, _, _ := unstructured.NestedString(got.Object, "spec", "package", "tag"); v != "1.1.0" {
		t.Errorf("tag = %q, want 1.1.0", v)
	}
	if v, _, _ := unstructured.NestedInt64(got.Object, "spec", "parameters", "replicas"); v != 2 {
		t.Errorf("replicas = %d, want 2", v)
	}
	if v, _, _ := unstructured.NestedString(got.Object, "spec", "parameters", "image"); v != "a" {
		t.Errorf("image = %q, want a", v)
	}
	if v, _, _ := unstructured.NestedBool(got.Object, "spec", "protected"); !v {
		t.Error("spec.protected was dropped")
	}
	if v, _, _ := unstructured.NestedStringSlice(got.Object, "spec", "roles"); len(v) != 1 || v[0] != "airflow" {
		t.Errorf("spec.roles = %v, want [airflow]", v)
	}
	if v, _, _ := unstructured.NestedString(got.Object, "spec", "extensions", "okdp.io", "databases", "metadata", "server"); v != "pg" {
		t.Errorf("spec.extensions lost, got server %q", v)
	}
	if v, _, _ := unstructured.NestedString(got.Object, "spec", "package", "secretRef", "name"); v != "pull" {
		t.Errorf("spec.package.secretRef lost, got %q", v)
	}
}
