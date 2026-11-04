package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/httpstream"
	apiremotecommand "k8s.io/apimachinery/pkg/util/remotecommand"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	k8sexec "k8s.io/client-go/util/exec"
)

// Client is the Kubernetes API as the activities use it: one call per API
// operation, no policy. Errors are the API server's, classified by the
// activities.
type Client interface {
	// Namespaced reports whether the kind lives in a namespace. A kind the
	// cluster does not serve is a meta.NoKindMatchError.
	Namespaced(gvk schema.GroupVersionKind) (bool, error)
	Create(ctx context.Context, obj *unstructured.Unstructured, manager string) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, obj *unstructured.Unstructured, manager string, force bool) (*unstructured.Unstructured, error)
	Get(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts metav1.DeleteOptions) error
	Exec(ctx context.Context, req ExecRequest) (*ExecResult, error)
	// GetSecret and ApplySecret move secret material inside the cluster. They
	// are not activities, so the material never enters a payload.
	GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error)
	ApplySecret(ctx context.Context, secret *corev1.Secret, manager string) error
}

// ExecRequest runs Command in Container of Pod. MaxOutputBytes caps stdout
// and stderr each; past it the output is cut and Truncated set.
type ExecRequest struct {
	Namespace      string
	Pod            string
	Container      string
	Command        []string
	Stdin          []byte
	MaxOutputBytes int64
}

// ExecResult is what the command wrote and how it exited. A non-zero exit is
// a result; an error means the exec itself did not run to completion.
type ExecResult struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int32
	Truncated bool
}

// LoadConfig reads a kubeconfig, or the in-cluster credentials when the path
// is empty. context selects a kubeconfig context; empty means its current one.
func LoadConfig(kubeconfigPath, context string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		return rest.InClusterConfig()
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: context},
	).ClientConfig()
}

// NewClient connects dynamic, typed and discovery clients over one config.
func NewClient(cfg *rest.Config) (Client, error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("typed client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(typed.Discovery()))
	return newClient(dyn, typed, mapper, cfg), nil
}

func newClient(dyn dynamic.Interface, typed kubernetes.Interface, mapper meta.RESTMapper, cfg *rest.Config) *client {
	return &client{dynamic: dyn, typed: typed, mapper: mapper, config: cfg}
}

type client struct {
	dynamic dynamic.Interface
	typed   kubernetes.Interface
	mapper  meta.RESTMapper
	config  *rest.Config
}

// mapping resolves a kind through discovery, refreshing once on a miss so a
// CRD applied earlier in the workflow is found.
func (c *client) mapping(gvk schema.GroupVersionKind) (*meta.RESTMapping, error) {
	m, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if meta.IsNoMatchError(err) {
		if r, ok := c.mapper.(meta.ResettableRESTMapper); ok {
			r.Reset()
			m, err = c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		}
	}
	return m, err
}

func (c *client) resource(m *meta.RESTMapping, namespace string) dynamic.ResourceInterface {
	if m.Scope.Name() == meta.RESTScopeNameNamespace {
		return c.dynamic.Resource(m.Resource).Namespace(namespace)
	}
	return c.dynamic.Resource(m.Resource)
}

func (c *client) Namespaced(gvk schema.GroupVersionKind) (bool, error) {
	m, err := c.mapping(gvk)
	if err != nil {
		return false, err
	}
	return m.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

func (c *client) Create(ctx context.Context, obj *unstructured.Unstructured, manager string) (*unstructured.Unstructured, error) {
	m, err := c.mapping(obj.GroupVersionKind())
	if err != nil {
		return nil, err
	}
	return c.resource(m, obj.GetNamespace()).Create(ctx, obj, metav1.CreateOptions{FieldManager: manager})
}

func (c *client) Apply(ctx context.Context, obj *unstructured.Unstructured, manager string, force bool) (*unstructured.Unstructured, error) {
	m, err := c.mapping(obj.GroupVersionKind())
	if err != nil {
		return nil, err
	}
	return c.resource(m, obj.GetNamespace()).Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{FieldManager: manager, Force: force})
}

func (c *client) Get(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (*unstructured.Unstructured, error) {
	m, err := c.mapping(gvk)
	if err != nil {
		return nil, err
	}
	return c.resource(m, namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *client) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts metav1.DeleteOptions) error {
	m, err := c.mapping(gvk)
	if err != nil {
		return err
	}
	return c.resource(m, namespace).Delete(ctx, name, opts)
}

func (c *client) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return c.typed.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *client) ApplySecret(ctx context.Context, secret *corev1.Secret, manager string) error {
	secret.APIVersion, secret.Kind = "v1", "Secret"
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	if err != nil {
		return err
	}
	_, err = c.Apply(ctx, &unstructured.Unstructured{Object: content}, manager, true)
	return err
}

// Exec runs the command over a WebSocket stream, falling back to SPDY where
// the API server cannot upgrade. The pod is read first so a missing pod or
// container is an API error rather than a stream failure.
func (c *client) Exec(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	pod, err := c.typed.CoreV1().Pods(req.Namespace).Get(ctx, req.Pod, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if pod.Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("pod %s/%s is %s, not Running", req.Namespace, req.Pod, pod.Status.Phase)
	}
	found := false
	for _, ct := range pod.Spec.Containers {
		found = found || ct.Name == req.Container
	}
	if !found {
		return nil, fmt.Errorf("pod %s/%s has no container %q", req.Namespace, req.Pod, req.Container)
	}

	request := c.typed.CoreV1().RESTClient().Post().
		Resource("pods").Name(req.Pod).Namespace(req.Namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: req.Container,
			Command:   req.Command,
			Stdin:     len(req.Stdin) > 0,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	ws, err := remotecommand.NewWebSocketExecutorForProtocols(c.config, "GET", request.URL().String(),
		apiremotecommand.StreamProtocolV5Name, apiremotecommand.StreamProtocolV4Name,
		apiremotecommand.StreamProtocolV3Name, apiremotecommand.StreamProtocolV2Name,
		apiremotecommand.StreamProtocolV1Name)
	if err != nil {
		return nil, fmt.Errorf("websocket executor: %w", err)
	}
	spdy, err := remotecommand.NewSPDYExecutor(c.config, "POST", request.URL())
	if err != nil {
		return nil, fmt.Errorf("spdy executor: %w", err)
	}
	executor, err := remotecommand.NewFallbackExecutor(ws, spdy, func(err error) bool {
		var upgrade *httpstream.UpgradeFailureError
		return errors.As(err, &upgrade)
	})
	if err != nil {
		return nil, fmt.Errorf("executor: %w", err)
	}

	stdout, stderr := newCappedWriter(req.MaxOutputBytes), newCappedWriter(req.MaxOutputBytes)
	var stdin io.Reader
	if len(req.Stdin) > 0 {
		stdin = bytes.NewReader(req.Stdin)
	}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: stderr})
	result := &ExecResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Truncated: stdout.truncated || stderr.truncated}
	var exit k8sexec.ExitError
	if errors.As(err, &exit) {
		result.ExitCode = int32(exit.ExitStatus())
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// cappedWriter keeps the first max bytes and drops the rest, so a chatty
// command cannot grow an activity result past Temporal's payload limit.
// The buffer is a field, not embedded: an embedded bytes.Buffer would lend
// its ReadFrom, and io.Copy would fill it past the cap without calling Write.
type cappedWriter struct {
	buf       bytes.Buffer
	max       int64
	truncated bool
}

func newCappedWriter(max int64) *cappedWriter { return &cappedWriter{max: max} }

func (w *cappedWriter) Write(p []byte) (int, error) {
	room := w.max - int64(w.buf.Len())
	if int64(len(p)) > room {
		w.truncated = true
		if room > 0 {
			w.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *cappedWriter) Bytes() []byte { return w.buf.Bytes() }
