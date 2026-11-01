package kube

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

var (
	gvkConfigMap = schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	gvkNamespace = schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}
	gvkWidget    = schema.GroupVersionKind{Group: "acme.io", Version: "v1", Kind: "Widget"}
)

// newFakeClient wires the client over in-memory API fakes and a static
// mapper that knows ConfigMap, Namespace and Secret.
func newFakeClient(objs ...runtime.Object) (*client, *k8sfake.Clientset) {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(gvkConfigMap, meta.RESTScopeNamespace)
	mapper.Add(gvkNamespace, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}, meta.RESTScopeNamespace)
	typed := k8sfake.NewClientset(objs...)
	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	return newClient(dyn, typed, mapper, &rest.Config{}), typed
}

func configMap(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("ConfigMap")
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func TestClient_Namespaced(t *testing.T) {
	c, _ := newFakeClient()
	tests := []struct {
		name    string
		gvk     schema.GroupVersionKind
		want    bool
		noMatch bool
	}{
		{name: "success - namespaced kind", gvk: gvkConfigMap, want: true},
		{name: "success - cluster-scoped kind", gvk: gvkNamespace, want: false},
		{name: "error - kind the cluster does not serve", gvk: gvkWidget, noMatch: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Namespaced(tt.gvk)
			if tt.noMatch {
				if !meta.IsNoMatchError(err) {
					t.Fatalf("want NoMatch, got %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("want %v, got %v, %v", tt.want, got, err)
			}
		})
	}
}

func TestClient_Create(t *testing.T) {
	ctx := context.Background()
	c, _ := newFakeClient()
	created, err := c.Create(ctx, configMap("a", "ns"), "m")
	if err != nil || created.GetName() != "a" {
		t.Fatalf("create: %v, %v", created, err)
	}
	got, err := c.Get(ctx, gvkConfigMap, "ns", "a")
	if err != nil || got.GetNamespace() != "ns" {
		t.Fatalf("get after create: %v, %v", got, err)
	}
	if _, err := c.Create(ctx, configMap("a", "ns"), "m"); !apierrors.IsAlreadyExists(err) {
		t.Fatalf("second create should be AlreadyExists, got %v", err)
	}
	if _, err := c.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "acme.io/v1", "kind": "Widget", "metadata": map[string]any{"name": "w"}}}, "m"); !meta.IsNoMatchError(err) {
		t.Fatalf("unknown kind should be NoMatch, got %v", err)
	}
}

func TestClient_Delete(t *testing.T) {
	ctx := context.Background()
	c, _ := newFakeClient()
	if _, err := c.Create(ctx, configMap("a", "ns"), "m"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, gvkConfigMap, "ns", "a", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.Get(ctx, gvkConfigMap, "ns", "a"); !apierrors.IsNotFound(err) {
		t.Fatalf("get after delete should be NotFound, got %v", err)
	}
	if err := c.Delete(ctx, gvkConfigMap, "ns", "a", metav1.DeleteOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("second delete should be NotFound, got %v", err)
	}
}

func TestClient_GetSecret(t *testing.T) {
	c, _ := newFakeClient(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "ns"}, Data: map[string][]byte{"k": []byte("v")}})
	got, err := c.GetSecret(context.Background(), "ns", "creds")
	if err != nil || string(got.Data["k"]) != "v" {
		t.Fatalf("get secret: %v, %v", got, err)
	}
	if _, err := c.GetSecret(context.Background(), "ns", "missing"); !apierrors.IsNotFound(err) {
		t.Fatalf("missing secret should be NotFound, got %v", err)
	}
}

func TestCappedWriter_Write(t *testing.T) {
	tests := []struct {
		name          string
		max           int64
		writes        []string
		want          string
		wantTruncated bool
	}{
		{name: "success - under the cap", max: 10, writes: []string{"abc", "def"}, want: "abcdef"},
		{name: "success - exactly the cap", max: 6, writes: []string{"abc", "def"}, want: "abcdef"},
		{name: "success - over the cap keeps the head", max: 4, writes: []string{"abc", "def"}, want: "abcd", wantTruncated: true},
		{name: "success - nothing after the cap", max: 2, writes: []string{"abc", "def"}, want: "ab", wantTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newCappedWriter(tt.max)
			for _, s := range tt.writes {
				if n, err := w.Write([]byte(s)); err != nil || n != len(s) {
					t.Fatalf("write reported %d, %v", n, err)
				}
			}
			if w.String() != tt.want || w.truncated != tt.wantTruncated {
				t.Fatalf("want %q truncated=%v, got %q truncated=%v", tt.want, tt.wantTruncated, w.String(), w.truncated)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	kubeconfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(kubeconfig, []byte(`apiVersion: v1
kind: Config
clusters:
- name: one
  cluster: {server: https://one.example:6443}
- name: two
  cluster: {server: https://two.example:6443}
contexts:
- name: one
  context: {cluster: one, user: u}
- name: two
  context: {cluster: two, user: u}
current-context: one
users:
- name: u
  user: {token: t}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		path     string
		context  string
		wantHost string
		wantErr  error
	}{
		{name: "success - current context", path: kubeconfig, wantHost: "https://one.example:6443"},
		{name: "success - named context", path: kubeconfig, context: "two", wantHost: "https://two.example:6443"},
		{name: "error - empty path means in-cluster, which this is not", wantErr: rest.ErrNotInCluster},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadConfig(tt.path, tt.context)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil || cfg.Host != tt.wantHost {
				t.Fatalf("want host %s, got %v, %v", tt.wantHost, cfg, err)
			}
		})
	}
}
