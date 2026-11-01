// Package kube implements kube.v1: Kubernetes writes, waits and exec over
// pre-rendered manifests, with the credentials of the worker that hosts it.
package kube

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	pb "github.com/metacensus/library-temporal-kube/gen/kube/v1"
)

const (
	// DefaultFieldManager names the server-side apply manager when the
	// consumer sets none. Two consumers writing one object should differ.
	DefaultFieldManager = "library-temporal-kube"

	// LabelManagedBy marks every object a write touched with the field
	// manager, so leaked objects can be listed.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// AnnotationWorkflow carries the workflow id that wrote the object.
	AnnotationWorkflow = "kube.temporal.metacensus.org/workflow-id"
	// AnnotationIdempotency marks an object with the run and activity that
	// created it, so a retried Create recognises its own earlier attempt.
	AnnotationIdempotency = "kube.temporal.metacensus.org/idempotency-key"

	defaultMaxOutputBytes = 512 << 10
)

// Activities implements the generated KubernetesActivities over a Client.
type Activities struct {
	client      Client
	manager     string
	labels      map[string]string
	annotations map[string]string
	decoder     runtime.Decoder
}

// Option configures Activities at construction; nothing here travels in a payload.
type Option func(*Activities)

// WithFieldManager names this worker's server-side apply manager.
func WithFieldManager(name string) Option { return func(a *Activities) { a.manager = name } }

// WithLabels adds labels to every object written.
func WithLabels(labels map[string]string) Option {
	return func(a *Activities) { a.labels = labels }
}

// WithAnnotations adds annotations to every object written.
func WithAnnotations(annotations map[string]string) Option {
	return func(a *Activities) { a.annotations = annotations }
}

// NewActivities builds the activity set; register it with RegisterActivities.
func NewActivities(client Client, opts ...Option) *Activities {
	a := &Activities{
		client:  client,
		manager: DefaultFieldManager,
		decoder: yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *Activities) Create(ctx context.Context, in *pb.CreateInput) (*pb.CreateOutput, error) {
	key := idempotencyKey(ctx)
	out := &pb.CreateOutput{}
	for _, r := range in.GetResources() {
		obj, err := a.prepare(ctx, r, in.GetNamespace(), in.GetAllowSecrets())
		if err != nil {
			return nil, err
		}
		if key != "" {
			annotate(obj, AnnotationIdempotency, key)
		}
		created, err := a.client.Create(ctx, obj, a.manager)
		if apierrors.IsAlreadyExists(err) && key != "" {
			if existing, gerr := a.client.Get(ctx, obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName()); gerr == nil && existing.GetAnnotations()[AnnotationIdempotency] == key {
				created, err = existing, nil
			}
		}
		if err != nil {
			return nil, apiError(r.GetId(), err)
		}
		out.Resources = append(out.Resources, identifier(r.GetId(), created))
	}
	return out, nil
}

func (a *Activities) Apply(ctx context.Context, in *pb.ApplyInput) (*pb.ApplyOutput, error) {
	out := &pb.ApplyOutput{}
	for _, r := range in.GetResources() {
		obj, err := a.prepare(ctx, r, in.GetNamespace(), in.GetAllowSecrets())
		if err != nil {
			return nil, err
		}
		applied, err := a.client.Apply(ctx, obj, a.manager, in.GetForce())
		if err != nil {
			return nil, apiError(r.GetId(), err)
		}
		out.Resources = append(out.Resources, identifier(r.GetId(), applied))
	}
	return out, nil
}

func (a *Activities) Delete(ctx context.Context, in *pb.DeleteInput) (*pb.DeleteOutput, error) {
	policy := propagation(in.GetPropagation())
	for _, id := range in.GetResources() {
		gvk, err := kind(id)
		if err != nil {
			return nil, err
		}
		opts := metav1.DeleteOptions{PropagationPolicy: &policy}
		if id.GetUid() != "" {
			uid := types.UID(id.GetUid())
			opts.Preconditions = &metav1.Preconditions{UID: &uid}
		}
		err = a.client.Delete(ctx, gvk, id.GetNamespace(), id.GetName(), opts)
		switch {
		case err == nil, apierrors.IsNotFound(err), meta.IsNoMatchError(err):
		case apierrors.IsConflict(err) && id.GetUid() != "":
			// A different object holds the name now; the one named is gone.
		default:
			return nil, apiError(id.GetId(), err)
		}
	}
	return &pb.DeleteOutput{}, nil
}

func (a *Activities) Status(ctx context.Context, in *pb.StatusInput) (*pb.StatusOutput, error) {
	statuses, err := a.statuses(ctx, in.GetResources())
	if err != nil {
		return nil, err
	}
	return &pb.StatusOutput{Resources: statuses}, nil
}

func (a *Activities) Ready(ctx context.Context, in *pb.ReadyInput) (*pb.ReadyOutput, error) {
	statuses, err := a.statuses(ctx, in.GetResources())
	if err != nil {
		return nil, err
	}
	var failed, pending []string
	for _, s := range statuses {
		switch s.GetCondition() {
		case pb.Condition_CONDITION_CURRENT:
		case pb.Condition_CONDITION_FAILED:
			failed = append(failed, describe(s))
		default:
			pending = append(pending, describe(s))
		}
	}
	if len(failed) > 0 {
		return nil, newError(ErrTypeFailed, false, nil, Detail{ID: firstID(statuses, pb.Condition_CONDITION_FAILED)}, "failed: %s", strings.Join(failed, "; "))
	}
	if len(pending) > 0 {
		return nil, newError(ErrTypeNotReady, true, nil, Detail{}, "not ready: %s", strings.Join(pending, "; "))
	}
	return &pb.ReadyOutput{}, nil
}

func (a *Activities) Gone(ctx context.Context, in *pb.GoneInput) (*pb.GoneOutput, error) {
	statuses, err := a.statuses(ctx, in.GetResources())
	if err != nil {
		return nil, err
	}
	var present []string
	for _, s := range statuses {
		if s.GetCondition() != pb.Condition_CONDITION_NOT_FOUND {
			present = append(present, describe(s))
		}
	}
	if len(present) > 0 {
		return nil, newError(ErrTypeExists, true, nil, Detail{}, "still present: %s", strings.Join(present, "; "))
	}
	return &pb.GoneOutput{}, nil
}

func (a *Activities) ExecPod(ctx context.Context, in *pb.ExecPodInput) (*pb.ExecPodOutput, error) {
	pod := in.GetPod()
	switch {
	case pod.GetNamespace() == "" || pod.GetName() == "":
		return nil, invalid(pod.GetId(), "pod needs namespace and name")
	case in.GetContainer() == "":
		return nil, invalid(pod.GetId(), "container is required")
	case len(in.GetCommand()) == 0:
		return nil, invalid(pod.GetId(), "command is required")
	}
	if d := in.GetTimeout().AsDuration(); d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	stop := heartbeat(ctx)
	defer stop()

	max := in.GetMaxOutputBytes()
	if max <= 0 {
		max = defaultMaxOutputBytes
	}
	result, err := a.client.Exec(ctx, ExecRequest{
		Namespace: pod.GetNamespace(), Pod: pod.GetName(), Container: in.GetContainer(),
		Command: in.GetCommand(), Stdin: in.GetStdin(), MaxOutputBytes: max,
	})
	switch {
	case err == nil:
	case apierrors.IsNotFound(err), apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return nil, apiError(pod.GetId(), err)
	default:
		return nil, newError(ErrTypeExec, true, err, Detail{ID: pod.GetId(), Kind: "Pod", Name: pod.GetName(), Namespace: pod.GetNamespace()}, "%s: exec: %v", pod.GetId(), err)
	}
	return &pb.ExecPodOutput{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode, Truncated: result.Truncated}, nil
}

// prepare decodes a manifest, refuses what must not cross history, settles
// the namespace, and stamps the ownership labels and annotations.
func (a *Activities) prepare(ctx context.Context, r *pb.Resource, namespace string, allowSecrets bool) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	_, gvk, err := a.decoder.Decode(r.GetManifestYaml(), nil, obj)
	if err != nil {
		return nil, invalid(r.GetId(), "decode manifest: %v", err)
	}
	if obj.GetName() == "" {
		return nil, invalid(r.GetId(), "manifest has no metadata.name")
	}
	if gvk.Group == "" && gvk.Kind == "Secret" && !allowSecrets {
		return nil, invalid(r.GetId(), "a Secret's data would be recorded in workflow history; set allow_secrets to carry it anyway")
	}
	namespaced, err := a.client.Namespaced(*gvk)
	if err != nil {
		return nil, apiError(r.GetId(), err)
	}
	if namespaced && obj.GetNamespace() == "" {
		if namespace == "" {
			return nil, invalid(r.GetId(), "%s is namespaced and neither the manifest nor the request names a namespace", gvk.Kind)
		}
		obj.SetNamespace(namespace)
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[LabelManagedBy] = a.manager
	for k, v := range a.labels {
		labels[k] = v
	}
	obj.SetLabels(labels)
	for k, v := range a.annotations {
		annotate(obj, k, v)
	}
	if activity.IsActivity(ctx) {
		annotate(obj, AnnotationWorkflow, activity.GetInfo(ctx).WorkflowExecution.ID)
	}
	return obj, nil
}

func (a *Activities) statuses(ctx context.Context, ids []*pb.ResourceIdentifier) ([]*pb.ResourceStatus, error) {
	out := make([]*pb.ResourceStatus, 0, len(ids))
	for _, id := range ids {
		gvk, err := kind(id)
		if err != nil {
			return nil, err
		}
		obj, err := a.client.Get(ctx, gvk, id.GetNamespace(), id.GetName())
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			out = append(out, &pb.ResourceStatus{Id: id.GetId(), Condition: pb.Condition_CONDITION_NOT_FOUND})
			continue
		}
		if err != nil {
			return nil, apiError(id.GetId(), err)
		}
		result, err := status.Compute(obj)
		if err != nil {
			return nil, invalid(id.GetId(), "compute status: %v", err)
		}
		c := condition(result.Status)
		// kstatus calls an exited Pod Current whichever way it exited; a Pod
		// that failed will not run without a change, which is what Failed means here.
		if phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase"); obj.GetKind() == "Pod" && phase == "Failed" {
			c = pb.Condition_CONDITION_FAILED
		}
		out = append(out, &pb.ResourceStatus{Id: id.GetId(), Condition: c, Message: result.Message, Generation: obj.GetGeneration()})
	}
	return out, nil
}

func condition(s status.Status) pb.Condition {
	switch s {
	case status.CurrentStatus:
		return pb.Condition_CONDITION_CURRENT
	case status.InProgressStatus:
		return pb.Condition_CONDITION_IN_PROGRESS
	case status.FailedStatus:
		return pb.Condition_CONDITION_FAILED
	case status.TerminatingStatus:
		return pb.Condition_CONDITION_TERMINATING
	case status.NotFoundStatus:
		return pb.Condition_CONDITION_NOT_FOUND
	default:
		return pb.Condition_CONDITION_UNSPECIFIED
	}
}

func propagation(p pb.Propagation) metav1.DeletionPropagation {
	switch p {
	case pb.Propagation_PROPAGATION_FOREGROUND:
		return metav1.DeletePropagationForeground
	case pb.Propagation_PROPAGATION_ORPHAN:
		return metav1.DeletePropagationOrphan
	default:
		return metav1.DeletePropagationBackground
	}
}

func kind(id *pb.ResourceIdentifier) (schema.GroupVersionKind, error) {
	if id.GetApiVersion() == "" || id.GetKind() == "" || id.GetName() == "" {
		return schema.GroupVersionKind{}, invalid(id.GetId(), "identifier needs api_version, kind and name")
	}
	return schema.FromAPIVersionAndKind(id.GetApiVersion(), id.GetKind()), nil
}

func identifier(id string, obj *unstructured.Unstructured) *pb.ResourceIdentifier {
	return &pb.ResourceIdentifier{
		Id:         id,
		ApiVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Uid:        string(obj.GetUID()),
		Generation: obj.GetGeneration(),
	}
}

func annotate(obj *unstructured.Unstructured, key, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}

// idempotencyKey is stable across the attempts of one activity execution and
// distinct between executions, which is what lets a retried Create tell its
// own earlier attempt from someone else's object.
func idempotencyKey(ctx context.Context) string {
	if !activity.IsActivity(ctx) {
		return ""
	}
	info := activity.GetInfo(ctx)
	return info.WorkflowExecution.RunID + "/" + info.ActivityID
}

// heartbeat keeps a long exec alive and lets cancellation reach it. Outside
// an activity it does nothing.
func heartbeat(ctx context.Context) (stop func()) {
	if !activity.IsActivity(ctx) {
		return func() {}
	}
	interval := activity.GetInfo(ctx).HeartbeatTimeout / 3
	if interval <= 0 {
		interval = 10 * time.Second
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				activity.RecordHeartbeat(ctx)
			}
		}
	}()
	return func() { close(done) }
}

func describe(s *pb.ResourceStatus) string {
	name := strings.TrimPrefix(s.GetCondition().String(), "CONDITION_")
	if s.GetMessage() == "" {
		return fmt.Sprintf("%s %s", s.GetId(), name)
	}
	return fmt.Sprintf("%s %s (%s)", s.GetId(), name, s.GetMessage())
}

func firstID(statuses []*pb.ResourceStatus, c pb.Condition) string {
	for _, s := range statuses {
		if s.GetCondition() == c {
			return s.GetId()
		}
	}
	return ""
}
