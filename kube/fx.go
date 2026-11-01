package kube

import (
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"

	pb "github.com/metacensus/library-temporal-kube/gen/kube/v1"
)

// Module provides the Client and Activities. The consumer provides the
// *rest.Config: credentials are the worker's, never the library's.
func Module(opts ...Option) fx.Option {
	return fx.Module("kube",
		fx.Provide(NewClient, func(c Client) *Activities { return NewActivities(c, opts...) }),
	)
}

// RegisterActivities registers kube.v1 on the worker.
func RegisterActivities(w worker.ActivityRegistry, a *Activities) {
	pb.RegisterKubernetesActivities(w, a)
}
