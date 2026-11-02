package kube_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	pb "github.com/metacensus/library-temporal-kube/gen/kube/v1"
	kube "github.com/metacensus/library-temporal-kube/lib/kube/v1"
	kubemocks "github.com/metacensus/library-temporal-kube/mocks/kube/v1"
)

var (
	gvkConfigMap  = schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}
	gvkNamespace  = schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}
	configmaps    = schema.GroupResource{Resource: "configmaps"}
	errNoMatch    = &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "acme.io", Kind: "Widget"}, SearchedVersions: []string{"v1"}}
	errTransient  = apierrors.NewServiceUnavailable("etcd leader changed")
	errExecBroken = errors.New("stream closed")
)

// run executes an activity method inside a Temporal test environment, so
// activity.GetInfo and heartbeats work as they do on a worker.
func run(t *testing.T, fn, in, out any) error {
	t.Helper()
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(fn)
	val, err := env.ExecuteActivity(fn, in)
	if err != nil {
		return err
	}
	return val.Get(out)
}

func assertError(t *testing.T, err error, errType string, retryable bool) {
	t.Helper()
	if errType == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("want ApplicationError %s, got %T: %v", errType, err, err)
	}
	if appErr.Type() != errType {
		t.Fatalf("want error type %s, got %s: %v", errType, appErr.Type(), err)
	}
	if appErr.NonRetryable() == retryable {
		t.Fatalf("want retryable=%v, got %v: %v", retryable, !appErr.NonRetryable(), err)
	}
}

func manifest(apiVersion, kind, name, namespace string) []byte {
	m := fmt.Sprintf("apiVersion: %s\nkind: %s\nmetadata:\n  name: %s\n", apiVersion, kind, name)
	if namespace != "" {
		m += "  namespace: " + namespace + "\n"
	}
	return []byte(m)
}

func object(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetUID(types.UID("uid-" + name))
	obj.SetGeneration(1)
	return obj
}

func resource(id string, m []byte) *pb.Resource {
	return &pb.Resource{Id: id, ManifestYaml: m}
}

func identifier(id, apiVersion, kind, name, namespace string) *pb.ResourceIdentifier {
	return &pb.ResourceIdentifier{Id: id, ApiVersion: apiVersion, Kind: kind, Name: name, Namespace: namespace}
}

func TestActivities_Create(t *testing.T) {
	tests := []struct {
		name      string
		in        *pb.CreateInput
		opts      []kube.Option
		setupMock func(m *kubemocks.MockClient)
		wantIDs   []string
		errType   string
		retryable bool
	}{
		{
			name:    "error - manifest does not parse",
			in:      &pb.CreateInput{Resources: []*pb.Resource{resource("a", []byte("{not yaml"))}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name:    "error - manifest has no name",
			in:      &pb.CreateInput{Resources: []*pb.Resource{resource("a", []byte("apiVersion: v1\nkind: ConfigMap\n"))}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name:    "error - a Secret is refused unless allowed",
			in:      &pb.CreateInput{Resources: []*pb.Resource{resource("s", manifest("v1", "Secret", "creds", "ns"))}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "error - the cluster serves no such kind",
			in:   &pb.CreateInput{Resources: []*pb.Resource{resource("w", manifest("acme.io/v1", "Widget", "w", "ns"))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(schema.GroupVersionKind{Group: "acme.io", Version: "v1", Kind: "Widget"}).Return(false, errNoMatch)
			},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "error - namespaced kind with no namespace anywhere",
			in:   &pb.CreateInput{Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", ""))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
			},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "success - request namespace, labels and annotations are stamped",
			in:   &pb.CreateInput{Namespace: "ns", Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", ""))}},
			opts: []kube.Option{kube.WithFieldManager("product"), kube.WithLabels(map[string]string{"team": "chain"})},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
				m.EXPECT().Create(mock.Anything, mock.MatchedBy(func(obj *unstructured.Unstructured) bool {
					return obj.GetNamespace() == "ns" &&
						obj.GetLabels()[kube.LabelManagedBy] == "product" &&
						obj.GetLabels()["team"] == "chain" &&
						obj.GetAnnotations()[kube.AnnotationWorkflow] != "" &&
						obj.GetAnnotations()[kube.AnnotationIdempotency] != ""
				}), "product").Return(object("v1", "ConfigMap", "a", "ns"), nil)
			},
			wantIDs: []string{"a"},
		},
		{
			name: "success - cluster-scoped kind needs no namespace",
			in:   &pb.CreateInput{Resources: []*pb.Resource{resource("n", manifest("v1", "Namespace", "team", ""))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkNamespace).Return(false, nil)
				m.EXPECT().Create(mock.Anything, mock.Anything, kube.DefaultFieldManager).Return(object("v1", "Namespace", "team", ""), nil)
			},
			wantIDs: []string{"n"},
		},
		{
			name: "success - a retry finds its own earlier attempt",
			in:   &pb.CreateInput{Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", "ns"))}},
			setupMock: func(m *kubemocks.MockClient) {
				var key string
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
				m.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, obj *unstructured.Unstructured, _ string) (*unstructured.Unstructured, error) {
					key = obj.GetAnnotations()[kube.AnnotationIdempotency]
					return nil, apierrors.NewAlreadyExists(configmaps, "a")
				})
				m.EXPECT().Get(mock.Anything, gvkConfigMap, "ns", "a").RunAndReturn(func(context.Context, schema.GroupVersionKind, string, string) (*unstructured.Unstructured, error) {
					existing := object("v1", "ConfigMap", "a", "ns")
					existing.SetAnnotations(map[string]string{kube.AnnotationIdempotency: key})
					return existing, nil
				})
			},
			wantIDs: []string{"a"},
		},
		{
			name: "error - someone else's object holds the name",
			in:   &pb.CreateInput{Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", "ns"))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
				m.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(nil, apierrors.NewAlreadyExists(configmaps, "a"))
				m.EXPECT().Get(mock.Anything, gvkConfigMap, "ns", "a").Return(object("v1", "ConfigMap", "a", "ns"), nil)
			},
			errType: kube.ErrTypeAlreadyExists,
		},
		{
			name: "error - forbidden is not retried",
			in:   &pb.CreateInput{Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", "ns"))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
				m.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(nil, apierrors.NewForbidden(configmaps, "a", errors.New("rbac")))
			},
			errType: kube.ErrTypeForbidden,
		},
		{
			name: "error - a transient API failure is retried and names the id it stopped at",
			in: &pb.CreateInput{Resources: []*pb.Resource{
				resource("first", manifest("v1", "ConfigMap", "first", "ns")),
				resource("second", manifest("v1", "ConfigMap", "second", "ns")),
			}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil).Times(2)
				m.EXPECT().Create(mock.Anything, mock.MatchedBy(named("first")), mock.Anything).Return(object("v1", "ConfigMap", "first", "ns"), nil)
				m.EXPECT().Create(mock.Anything, mock.MatchedBy(named("second")), mock.Anything).Return(nil, errTransient)
			},
			errType:   kube.ErrTypeUnavailable,
			retryable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			if tt.setupMock != nil {
				tt.setupMock(m)
			}
			a := kube.NewActivities(m, tt.opts...)
			var out pb.CreateOutput
			err := run(t, a.Create, tt.in, &out)
			assertError(t, err, tt.errType, tt.retryable)
			if tt.errType != "" {
				if tt.name == "error - a transient API failure is retried and names the id it stopped at" && !strings.Contains(err.Error(), "second") {
					t.Fatalf("error should name the failing id: %v", err)
				}
				return
			}
			assertIDs(t, out.GetResources(), tt.wantIDs)
		})
	}
}

func TestActivities_Apply(t *testing.T) {
	tests := []struct {
		name      string
		in        *pb.ApplyInput
		opts      []kube.Option
		setupMock func(m *kubemocks.MockClient)
		errType   string
	}{
		{
			name: "success - force and the field manager reach the client, identity comes back",
			in:   &pb.ApplyInput{Force: true, Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", "ns"))}},
			opts: []kube.Option{kube.WithFieldManager("product")},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
				applied := object("v1", "ConfigMap", "a", "ns")
				applied.SetGeneration(7)
				m.EXPECT().Apply(mock.Anything, mock.MatchedBy(named("a")), "product", true).Return(applied, nil)
			},
		},
		{
			name: "error - a field another manager owns",
			in:   &pb.ApplyInput{Resources: []*pb.Resource{resource("a", manifest("v1", "ConfigMap", "a", "ns"))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(gvkConfigMap).Return(true, nil)
				m.EXPECT().Apply(mock.Anything, mock.Anything, kube.DefaultFieldManager, false).Return(nil, apierrors.NewConflict(configmaps, "a", errors.New("field data.key is owned by kubectl")))
			},
			errType: kube.ErrTypeConflict,
		},
		{
			name:    "error - a Secret is refused unless allowed",
			in:      &pb.ApplyInput{Resources: []*pb.Resource{resource("s", manifest("v1", "Secret", "creds", "ns"))}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "success - a Secret passes when allowed",
			in:   &pb.ApplyInput{AllowSecrets: true, Resources: []*pb.Resource{resource("s", manifest("v1", "Secret", "creds", "ns"))}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Namespaced(schema.GroupVersionKind{Version: "v1", Kind: "Secret"}).Return(true, nil)
				m.EXPECT().Apply(mock.Anything, mock.Anything, mock.Anything, false).Return(object("v1", "Secret", "creds", "ns"), nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			if tt.setupMock != nil {
				tt.setupMock(m)
			}
			a := kube.NewActivities(m, tt.opts...)
			var out pb.ApplyOutput
			err := run(t, a.Apply, tt.in, &out)
			assertError(t, err, tt.errType, false)
			if tt.errType != "" {
				return
			}
			got := out.GetResources()[0]
			if got.GetUid() == "" || got.GetKind() == "" || got.GetNamespace() != "ns" {
				t.Fatalf("identifier should carry kind, namespace and uid: %v", got)
			}
			if tt.name == "success - force and the field manager reach the client, identity comes back" && got.GetGeneration() != 7 {
				t.Fatalf("identifier should carry the written generation, got %d", got.GetGeneration())
			}
		})
	}
}

func TestActivities_Delete(t *testing.T) {
	foreground := metav1.DeletePropagationForeground
	tests := []struct {
		name      string
		in        *pb.DeleteInput
		setupMock func(m *kubemocks.MockClient)
		errType   string
		retryable bool
	}{
		{
			name:    "error - identifier without kind",
			in:      &pb.DeleteInput{Resources: []*pb.ResourceIdentifier{{Id: "a", Name: "a"}}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "success - propagation and uid precondition reach the client",
			in: &pb.DeleteInput{Propagation: pb.Propagation_PROPAGATION_FOREGROUND, Resources: []*pb.ResourceIdentifier{
				{Id: "a", ApiVersion: "v1", Kind: "ConfigMap", Name: "a", Namespace: "ns", Uid: "u1"},
			}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Delete(mock.Anything, gvkConfigMap, "ns", "a", mock.MatchedBy(func(o metav1.DeleteOptions) bool {
					return *o.PropagationPolicy == foreground && o.Preconditions != nil && string(*o.Preconditions.UID) == "u1"
				})).Return(nil)
			},
		},
		{
			name: "success - absent is success",
			in:   &pb.DeleteInput{Resources: []*pb.ResourceIdentifier{identifier("a", "v1", "ConfigMap", "a", "ns")}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Delete(mock.Anything, gvkConfigMap, "ns", "a", mock.Anything).Return(apierrors.NewNotFound(configmaps, "a"))
			},
		},
		{
			name: "success - another object under the name is left alone",
			in: &pb.DeleteInput{Resources: []*pb.ResourceIdentifier{
				{Id: "a", ApiVersion: "v1", Kind: "ConfigMap", Name: "a", Namespace: "ns", Uid: "u1"},
			}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Delete(mock.Anything, gvkConfigMap, "ns", "a", mock.Anything).Return(apierrors.NewConflict(configmaps, "a", errors.New("uid mismatch")))
			},
		},
		{
			name: "error - forbidden",
			in:   &pb.DeleteInput{Resources: []*pb.ResourceIdentifier{identifier("a", "v1", "ConfigMap", "a", "ns")}},
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Delete(mock.Anything, gvkConfigMap, "ns", "a", mock.Anything).Return(apierrors.NewForbidden(configmaps, "a", errors.New("rbac")))
			},
			errType: kube.ErrTypeForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			if tt.setupMock != nil {
				tt.setupMock(m)
			}
			var out pb.DeleteOutput
			err := run(t, kube.NewActivities(m).Delete, tt.in, &out)
			assertError(t, err, tt.errType, tt.retryable)
		})
	}
}

func TestActivities_Status(t *testing.T) {
	id := identifier("d", "apps/v1", "Deployment", "d", "ns")
	tests := []struct {
		name      string
		setupMock func(m *kubemocks.MockClient)
		want      pb.Condition
		errType   string
		retryable bool
	}{
		{
			name: "success - not found",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "d"))
			},
			want: pb.Condition_CONDITION_NOT_FOUND,
		},
		{
			name: "success - current once the written generation is observed and rolled out",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(deployment(2, 2, 1), nil)
			},
			want: pb.Condition_CONDITION_CURRENT,
		},
		{
			name: "success - in progress while the controller lags the generation",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(deployment(3, 2, 1), nil)
			},
			want: pb.Condition_CONDITION_IN_PROGRESS,
		},
		{
			name: "success - terminating",
			setupMock: func(m *kubemocks.MockClient) {
				obj := deployment(2, 2, 1)
				now := metav1.Now()
				obj.SetDeletionTimestamp(&now)
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(obj, nil)
			},
			want: pb.Condition_CONDITION_TERMINATING,
		},
		{
			name: "error - the API is unavailable",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(nil, errTransient)
			},
			errType:   kube.ErrTypeUnavailable,
			retryable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			tt.setupMock(m)
			var out pb.StatusOutput
			err := run(t, kube.NewActivities(m).Status, &pb.StatusInput{Resources: []*pb.ResourceIdentifier{id}}, &out)
			assertError(t, err, tt.errType, tt.retryable)
			if tt.errType != "" {
				return
			}
			if got := out.GetResources()[0]; got.GetId() != "d" || got.GetCondition() != tt.want {
				t.Fatalf("want %s for d, got %v", tt.want, got)
			}
		})
	}
}

func TestActivities_Ready(t *testing.T) {
	d := identifier("d", "apps/v1", "Deployment", "d", "ns")
	p := identifier("p", "v1", "Pod", "p", "ns")
	tests := []struct {
		name      string
		setupMock func(m *kubemocks.MockClient)
		errType   string
		retryable bool
	}{
		{
			name: "success - every resource current",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(deployment(1, 1, 1), nil)
				m.EXPECT().Get(mock.Anything, gvkPod, "ns", "p").Return(pod("Running", true), nil)
			},
		},
		{
			name: "error - one still in progress is retried",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(deployment(1, 1, 1), nil)
				m.EXPECT().Get(mock.Anything, gvkPod, "ns", "p").Return(pod("Pending", false), nil)
			},
			errType:   kube.ErrTypeNotReady,
			retryable: true,
		},
		{
			name: "error - one not yet found is retried",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "d"))
				m.EXPECT().Get(mock.Anything, gvkPod, "ns", "p").Return(pod("Running", true), nil)
			},
			errType:   kube.ErrTypeNotReady,
			retryable: true,
		},
		{
			name: "error - a failed resource is not retried",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(deployment(1, 1, 1), nil)
				m.EXPECT().Get(mock.Anything, gvkPod, "ns", "p").Return(pod("Failed", false), nil)
			},
			errType: kube.ErrTypeFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			tt.setupMock(m)
			var out pb.ReadyOutput
			err := run(t, kube.NewActivities(m).Ready, &pb.ReadyInput{Resources: []*pb.ResourceIdentifier{d, p}}, &out)
			assertError(t, err, tt.errType, tt.retryable)
		})
	}
}

func TestActivities_Gone(t *testing.T) {
	d := identifier("d", "apps/v1", "Deployment", "d", "ns")
	tests := []struct {
		name      string
		setupMock func(m *kubemocks.MockClient)
		errType   string
		retryable bool
	}{
		{
			name: "success - not found",
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "d"))
			},
		},
		{
			name: "error - still terminating is retried",
			setupMock: func(m *kubemocks.MockClient) {
				obj := deployment(1, 1, 1)
				now := metav1.Now()
				obj.SetDeletionTimestamp(&now)
				m.EXPECT().Get(mock.Anything, gvkDeployment, "ns", "d").Return(obj, nil)
			},
			errType:   kube.ErrTypeExists,
			retryable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			tt.setupMock(m)
			var out pb.GoneOutput
			err := run(t, kube.NewActivities(m).Gone, &pb.GoneInput{Resources: []*pb.ResourceIdentifier{d}}, &out)
			assertError(t, err, tt.errType, tt.retryable)
		})
	}
}

func TestActivities_ExecPod(t *testing.T) {
	podRef := &pb.ResourceIdentifier{Id: "p", Namespace: "ns", Name: "p"}
	base := func() *pb.ExecPodInput {
		return &pb.ExecPodInput{Pod: podRef, Container: "main", Command: []string{"sh", "-c", "echo hi"}}
	}
	tests := []struct {
		name      string
		in        *pb.ExecPodInput
		setupMock func(m *kubemocks.MockClient)
		want      *pb.ExecPodOutput
		errType   string
		retryable bool
	}{
		{
			name:    "error - pod without namespace",
			in:      &pb.ExecPodInput{Pod: &pb.ResourceIdentifier{Name: "p"}, Container: "main", Command: []string{"true"}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name:    "error - container is required",
			in:      &pb.ExecPodInput{Pod: podRef, Command: []string{"true"}},
			errType: kube.ErrTypeInvalid,
		},
		{
			name:    "error - command is required",
			in:      &pb.ExecPodInput{Pod: podRef, Container: "main"},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "success - output and the default cap reach the caller",
			in:   base(),
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Exec(mock.Anything, mock.MatchedBy(func(r kube.ExecRequest) bool {
					return r.Namespace == "ns" && r.Pod == "p" && r.Container == "main" && r.MaxOutputBytes == 512<<10
				})).Return(&kube.ExecResult{Stdout: []byte("hi\n")}, nil)
			},
			want: &pb.ExecPodOutput{Stdout: []byte("hi\n")},
		},
		{
			name: "success - a non-zero exit is a result",
			in:   base(),
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Exec(mock.Anything, mock.Anything).Return(&kube.ExecResult{Stderr: []byte("no such file"), ExitCode: 2, Truncated: true}, nil)
			},
			want: &pb.ExecPodOutput{Stderr: []byte("no such file"), ExitCode: 2, Truncated: true},
		},
		{
			name: "error - the pod does not exist",
			in:   base(),
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Exec(mock.Anything, mock.Anything).Return(nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "p"))
			},
			errType: kube.ErrTypeInvalid,
		},
		{
			name: "error - the exec itself fails",
			in:   base(),
			setupMock: func(m *kubemocks.MockClient) {
				m.EXPECT().Exec(mock.Anything, mock.Anything).Return(nil, errExecBroken)
			},
			errType:   kube.ErrTypeExec,
			retryable: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := kubemocks.NewMockClient(t)
			if tt.setupMock != nil {
				tt.setupMock(m)
			}
			var out pb.ExecPodOutput
			err := run(t, kube.NewActivities(m).ExecPod, tt.in, &out)
			assertError(t, err, tt.errType, tt.retryable)
			if tt.errType != "" {
				return
			}
			if string(out.GetStdout()) != string(tt.want.GetStdout()) || string(out.GetStderr()) != string(tt.want.GetStderr()) ||
				out.GetExitCode() != tt.want.GetExitCode() || out.GetTruncated() != tt.want.GetTruncated() {
				t.Fatalf("want %v, got %v", tt.want, &out)
			}
		})
	}
}

var (
	gvkDeployment = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	gvkPod        = schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
)

func named(name string) func(*unstructured.Unstructured) bool {
	return func(obj *unstructured.Unstructured) bool { return obj.GetName() == name }
}

func assertIDs(t *testing.T, got []*pb.ResourceIdentifier, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("want ids %v, got %v", want, got)
	}
	for i, id := range got {
		if id.GetId() != want[i] || id.GetUid() == "" {
			t.Fatalf("want ids %v with uids, got %v", want, got)
		}
	}
}

// deployment is a Deployment as kstatus reads it: current when the observed
// generation matches and every replica is updated, ready and available.
func deployment(generation, observed, replicas int64) *unstructured.Unstructured {
	obj := object("apps/v1", "Deployment", "d", "ns")
	obj.SetGeneration(generation)
	obj.Object["spec"] = map[string]any{"replicas": replicas}
	obj.Object["status"] = map[string]any{
		"observedGeneration": observed,
		"replicas":           replicas,
		"updatedReplicas":    replicas,
		"readyReplicas":      replicas,
		"availableReplicas":  replicas,
		"conditions": []any{
			map[string]any{"type": "Available", "status": "True"},
			map[string]any{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable"},
		},
	}
	return obj
}

func pod(phase string, ready bool) *unstructured.Unstructured {
	obj := object("v1", "Pod", "p", "ns")
	obj.Object["status"] = map[string]any{
		"phase":      phase,
		"conditions": []any{map[string]any{"type": "Ready", "status": map[bool]string{true: "True", false: "False"}[ready]}},
		"containerStatuses": []any{
			map[string]any{"name": "main", "ready": ready, "state": map[string]any{"running": map[string]any{}}},
		},
	}
	return obj
}
