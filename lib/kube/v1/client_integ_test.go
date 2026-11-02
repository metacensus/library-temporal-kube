//go:build integration

package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/tools/clientcmd"
)

// startK3s runs a k3s server in a container and returns a client over it.
func startK3s(t *testing.T) Client {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "rancher/k3s:v1.31.4-k3s1",
			ExposedPorts: []string{"6443/tcp"},
			Env:          map[string]string{"K3S_KUBECONFIG_OUTPUT": "/output/kubeconfig.yaml", "K3S_KUBECONFIG_MODE": "666"},
			Cmd:          []string{"server", "--disable=traefik", "--disable=metrics-server", "--tls-san=127.0.0.1"},
			Privileged:   true,
			WaitingFor:   wait.ForLog("Node controller sync successful").WithStartupTimeout(3 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("start k3s: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			if logs, err := container.Logs(ctx); err == nil {
				defer logs.Close()
				buf := make([]byte, 64<<10)
				n, _ := logs.Read(buf)
				t.Logf("k3s logs:\n%s", buf[:n])
			}
		}
		_ = container.Terminate(ctx)
	})

	reader, err := container.CopyFileFromContainer(ctx, "/output/kubeconfig.yaml")
	if err != nil {
		t.Fatalf("copy kubeconfig: %v", err)
	}
	defer reader.Close()
	raw, err := os.ReadFile("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 8<<10)
	for {
		chunk := make([]byte, 4<<10)
		n, rerr := reader.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if rerr != nil {
			break
		}
	}
	raw = append(raw, buf...)
	kubeconfig, err := clientcmd.Load(raw)
	if err != nil {
		t.Fatalf("parse kubeconfig: %v", err)
	}
	port, err := container.MappedPort(ctx, "6443")
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}
	for _, cluster := range kubeconfig.Clusters {
		cluster.Server = fmt.Sprintf("https://127.0.0.1:%s", port.Port())
		cluster.InsecureSkipTLSVerify = true
		cluster.CertificateAuthorityData = nil
	}
	path := filepath.Join(t.TempDir(), "kubeconfig.yaml")
	if err := clientcmd.WriteToFile(*kubeconfig, path); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	cfg, err := LoadConfig(path, "")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func decode(t *testing.T, manifest string) *unstructured.Unstructured {
	t.Helper()
	obj := &unstructured.Unstructured{}
	if _, _, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode([]byte(manifest), nil, obj); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return obj
}

func TestClient_RealCluster(t *testing.T) {
	c := startK3s(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	t.Run("discovery scopes kinds and refuses unknown ones", func(t *testing.T) {
		if namespaced, err := c.Namespaced(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}); err != nil || !namespaced {
			t.Fatalf("ConfigMap: %v, %v", namespaced, err)
		}
		if namespaced, err := c.Namespaced(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}); err != nil || namespaced {
			t.Fatalf("Namespace: %v, %v", namespaced, err)
		}
		if _, err := c.Namespaced(schema.GroupVersionKind{Group: "acme.io", Version: "v1", Kind: "Widget"}); !meta.IsNoMatchError(err) {
			t.Fatalf("Widget should be NoMatch, got %v", err)
		}
	})

	t.Run("create, apply, get and delete a ConfigMap", func(t *testing.T) {
		if _, err := c.Create(ctx, decode(t, "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: integ\n"), "test"); err != nil {
			t.Fatalf("create namespace: %v", err)
		}
		cm := decode(t, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n  namespace: integ\ndata:\n  k: one\n")
		created, err := c.Create(ctx, cm, "test")
		if err != nil || created.GetUID() == "" {
			t.Fatalf("create: %v, %v", created, err)
		}
		if _, err := c.Create(ctx, cm, "test"); !apierrors.IsAlreadyExists(err) {
			t.Fatalf("second create should be AlreadyExists, got %v", err)
		}

		cm.Object["data"] = map[string]any{"k": "two"}
		if _, err := c.Apply(ctx, cm, "other", false); !apierrors.IsConflict(err) {
			t.Fatalf("apply by another manager over a created field should conflict, got %v", err)
		}
		applied, err := c.Apply(ctx, cm, "other", true)
		if err != nil {
			t.Fatalf("forced apply: %v", err)
		}
		if v, _, _ := unstructured.NestedString(applied.Object, "data", "k"); v != "two" {
			t.Fatalf("apply did not land: %v", applied.Object["data"])
		}
		got, err := c.Get(ctx, schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, "integ", "cm")
		if err != nil || got.GetUID() != created.GetUID() {
			t.Fatalf("get: %v, %v", got, err)
		}
		if err := c.Delete(ctx, schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, "integ", "cm", metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := c.Get(ctx, schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, "integ", "cm"); !apierrors.IsNotFound(err) {
			t.Fatalf("get after delete should be NotFound, got %v", err)
		}
	})

	t.Run("secrets move in-cluster", func(t *testing.T) {
		if err := c.ApplySecret(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "integ"},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"k": []byte("v")},
		}, "test"); err != nil {
			t.Fatalf("apply secret: %v", err)
		}
		got, err := c.GetSecret(ctx, "integ", "creds")
		if err != nil || string(got.Data["k"]) != "v" {
			t.Fatalf("get secret: %v, %v", got, err)
		}
	})

	t.Run("exec returns output, exit code and truncation", func(t *testing.T) {
		pod := decode(t, "apiVersion: v1\nkind: Pod\nmetadata:\n  name: shell\n  namespace: integ\nspec:\n  containers:\n  - name: main\n    image: busybox:1.36\n    command: [\"sleep\", \"600\"]\n")
		if _, err := c.Create(ctx, pod, "test"); err != nil {
			t.Fatalf("create pod: %v", err)
		}
		deadline := time.Now().Add(3 * time.Minute)
		for {
			got, err := c.Get(ctx, schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, "integ", "shell")
			if err != nil {
				t.Fatalf("get pod: %v", err)
			}
			if phase, _, _ := unstructured.NestedString(got.Object, "status", "phase"); phase == "Running" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("pod never ran: %v", got.Object["status"])
			}
			time.Sleep(2 * time.Second)
		}
		res, err := c.Exec(ctx, ExecRequest{Namespace: "integ", Pod: "shell", Container: "main", Command: []string{"sh", "-c", "echo out; echo err >&2; exit 3"}, MaxOutputBytes: 1 << 10})
		if err != nil {
			t.Fatalf("exec: %v", err)
		}
		if string(res.Stdout) != "out\n" || string(res.Stderr) != "err\n" || res.ExitCode != 3 || res.Truncated {
			t.Fatalf("unexpected result: %+v", res)
		}
		res, err = c.Exec(ctx, ExecRequest{Namespace: "integ", Pod: "shell", Container: "main", Command: []string{"cat"}, Stdin: []byte("0123456789"), MaxOutputBytes: 4})
		if err != nil || string(res.Stdout) != "0123" || !res.Truncated {
			t.Fatalf("capped exec: %+v, %v", res, err)
		}
		if _, err := c.Exec(ctx, ExecRequest{Namespace: "integ", Pod: "shell", Container: "nope", Command: []string{"true"}}); err == nil {
			t.Fatal("exec into a missing container should fail")
		}
	})
}
