package kube

import (
	"fmt"

	"go.temporal.io/sdk/temporal"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
)

// Error types, carried as the ApplicationError type so a workflow can branch
// without parsing messages. The retryable ones are how Ready and Gone wait.
const (
	ErrTypeInvalid       = "Kube.Invalid"
	ErrTypeForbidden     = "Kube.Forbidden"
	ErrTypeConflict      = "Kube.Conflict"
	ErrTypeAlreadyExists = "Kube.AlreadyExists"
	ErrTypeUnavailable   = "Kube.Unavailable"
	ErrTypeExec          = "Kube.Exec"
	ErrTypeNotReady      = "Kube.NotReady"
	ErrTypeExists        = "Kube.Exists"
	ErrTypeFailed        = "Kube.Failed"
)

// Detail rides in the error's details and names the resource the activity
// stopped at.
type Detail struct {
	ID        string
	Kind      string
	Name      string
	Namespace string
	Reason    string
}

func newError(errType string, retryable bool, cause error, detail Detail, format string, args ...any) error {
	return temporal.NewApplicationErrorWithOptions(fmt.Sprintf(format, args...), errType, temporal.ApplicationErrorOptions{
		NonRetryable: !retryable,
		Cause:        cause,
		Details:      []any{detail},
	})
}

func invalid(id, format string, args ...any) error {
	return newError(ErrTypeInvalid, false, nil, Detail{ID: id}, "%s: %s", id, fmt.Sprintf(format, args...))
}

// apiError classifies an API server error: what the same request would
// produce again is not retried.
func apiError(id string, err error) error {
	detail := Detail{ID: id, Reason: string(apierrors.ReasonForError(err))}
	if st, ok := err.(apierrors.APIStatus); ok && st.Status().Details != nil {
		detail.Kind = st.Status().Details.Kind
		detail.Name = st.Status().Details.Name
	}
	msg := "%s: %v"
	switch {
	case meta.IsNoMatchError(err):
		return newError(ErrTypeInvalid, false, err, detail, "%s: the cluster serves no such kind; apply its CRD and wait on it first: %v", id, err)
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err), apierrors.IsNotFound(err), apierrors.IsMethodNotSupported(err):
		return newError(ErrTypeInvalid, false, err, detail, msg, id, err)
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return newError(ErrTypeForbidden, false, err, detail, msg, id, err)
	case apierrors.IsConflict(err):
		return newError(ErrTypeConflict, false, err, detail, msg, id, err)
	case apierrors.IsAlreadyExists(err):
		return newError(ErrTypeAlreadyExists, false, err, detail, msg, id, err)
	default:
		return newError(ErrTypeUnavailable, true, err, detail, msg, id, err)
	}
}
