# library-temporal-kube

Temporal activities for Kubernetes and Helm, shared by MetaCensus workers.
`helm.v1.RenderChart` renders a chart from an `fs.FS` the worker was built
with; `kube.v1` creates, applies, deletes, waits on and execs into resources
with the credentials of the worker that hosts it. The contract is the proto
under [proto/](proto/README.md); the design is declared in [twf/](twf/).

Everything sits at `<category>/<application space>/<version>`: `proto`,
`gen`, `lib`, `mocks` and `twf`, each over `kube/v1` and `helm/v1`. A
consumer provides a `*rest.Config` and an `fs.FS` to fx, includes
`kube.Module()` and `helm.Module` from `lib`, and registers the activities on
the worker it chooses. Payloads are workflow history: the library never puts secret
material in one and refuses to carry a `kind: Secret` unless told to.

`make check` is the gate; its integration tests start k3s in Docker.
Authoring follows the Temporal skill in
[metacensus/skills](https://github.com/metacensus/skills).
