package helm

import (
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"

	pb "github.com/metacensus/library-temporal-kube/gen/helm/v1"
)

// Module provides the Activities. The consumer provides the fs.FS of charts,
// so a worker cannot register the activity without the charts it names.
var Module = fx.Module("helm", fx.Provide(NewActivities))

// RegisterActivities registers helm.v1 on the worker.
func RegisterActivities(w worker.ActivityRegistry, a *Activities) {
	pb.RegisterHelmActivities(w, a)
}
