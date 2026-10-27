package repository

import (
	"context"
	"fmt"

	"github.com/okdp/okdp-control-plane-server/internal/repository/crd"
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

// ServiceRepository defines Kubernetes operations for KuboCD Release resources.
// All operations take an explicit namespace so releases live in the project namespace.
type ServiceRepository interface {
	Create(ctx context.Context, namespace string, release *crd.Release) error
	Get(ctx context.Context, namespace, name string) (*crd.Release, error)
	List(ctx context.Context, namespace, project string) ([]crd.Release, error)
	Update(ctx context.Context, namespace string, release *crd.Release) error
	Delete(ctx context.Context, namespace, name string) error
	Watch(ctx context.Context, namespace, project string) (watch.Interface, error)
}

type k8sServiceRepository struct {
	client dynamic.Interface
	gvr    schema.GroupVersionResource
}

func NewServiceRepository(client dynamic.Interface) ServiceRepository {
	return &k8sServiceRepository{
		client: client,
		gvr:    crd.GetReleaseGVR(),
	}
}

func (r *k8sServiceRepository) Create(ctx context.Context, namespace string, release *crd.Release) error {
	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(release)
	if err != nil {
		return fmt.Errorf("failed to convert Release to unstructured: %w", err)
	}

	_, err = r.client.Resource(r.gvr).Namespace(namespace).Create(
		ctx, &unstructured.Unstructured{Object: unstructuredMap}, metav1.CreateOptions{},
	)
	return err
}

func (r *k8sServiceRepository) Get(ctx context.Context, namespace, name string) (*crd.Release, error) {
	u, err := r.client.Resource(r.gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var release crd.Release
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &release); err != nil {
		return nil, fmt.Errorf("failed to convert from unstructured: %w", err)
	}
	return &release, nil
}

func (r *k8sServiceRepository) List(ctx context.Context, namespace, project string) ([]crd.Release, error) {
	opts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", crd.LabelProject, project),
	}

	list, err := r.client.Resource(r.gvr).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}

	var releases []crd.Release
	for _, item := range list.Items {
		var release crd.Release
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &release); err != nil {
			logrus.WithError(err).Warn("Failed to convert Release from unstructured")
			continue
		}
		releases = append(releases, release)
	}
	return releases, nil
}

func (r *k8sServiceRepository) Delete(ctx context.Context, namespace, name string) error {
	return r.client.Resource(r.gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// Fields of the Release spec the server manages. Update writes only these and
// keeps everything else (extensions, protected, roles, ...) as found.
var (
	managedSpecFields    = []string{"description", "parameters", "contexts", "targetNamespace", "createNamespace"}
	managedPackageFields = []string{"repository", "tag", "interval", "timeout", "insecure"}
)

func (r *k8sServiceRepository) Update(ctx context.Context, namespace string, release *crd.Release) error {
	desired, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&release.Spec)
	if err != nil {
		return fmt.Errorf("failed to convert Release to unstructured: %w", err)
	}

	existing, err := r.client.Resource(r.gvr).Namespace(namespace).Get(ctx, release.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	obj := existing.DeepCopy()
	obj.SetLabels(release.Labels)
	obj.SetAnnotations(release.Annotations)

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		spec = map[string]any{}
	}
	mergeFields(spec, desired, managedSpecFields)
	pkg, _, _ := unstructured.NestedMap(spec, "package")
	if pkg == nil {
		pkg = map[string]any{}
	}
	desiredPkg, _ := desired["package"].(map[string]any)
	mergeFields(pkg, desiredPkg, managedPackageFields)
	spec["package"] = pkg
	obj.Object["spec"] = spec

	_, err = r.client.Resource(r.gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// mergeFields copies keys from src into dst, deleting from dst those absent in src.
func mergeFields(dst, src map[string]any, keys []string) {
	for _, k := range keys {
		if v, ok := src[k]; ok {
			dst[k] = v
		} else {
			delete(dst, k)
		}
	}
}

func (r *k8sServiceRepository) Watch(ctx context.Context, namespace, project string) (watch.Interface, error) {
	opts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", crd.LabelProject, project),
	}
	return r.client.Resource(r.gvr).Namespace(namespace).Watch(ctx, opts)
}
