// Package helm implements helm.v1: rendering charts from an fs.FS the worker
// was built with, into resources for kube. It touches no cluster.
package helm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go.temporal.io/sdk/temporal"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/releaseutil"
	"sigs.k8s.io/yaml"

	pb "github.com/metacensus/library-temporal-kube/gen/helm/v1"
	kubev1 "github.com/metacensus/library-temporal-kube/gen/kube/v1"
)

// Error types. Neither is retried: the same inputs render the same way.
const (
	ErrTypeChartNotFound = "Helm.ChartNotFound"
	ErrTypeInvalid       = "Helm.Invalid"
)

// Activities implements the generated HelmActivities over a chart filesystem.
type Activities struct {
	charts fs.FS
}

// NewActivities renders charts found as directories of charts.
func NewActivities(charts fs.FS) *Activities {
	return &Activities{charts: charts}
}

func (a *Activities) RenderChart(ctx context.Context, in *pb.RenderChartInput) (*pb.RenderChartOutput, error) {
	if in.GetChart() == "" {
		return nil, invalid("chart is required")
	}
	if in.GetReleaseName() == "" {
		return nil, invalid("release_name is required")
	}
	ch, err := load(a.charts, in.GetChart())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, temporal.NewNonRetryableApplicationError(fmt.Sprintf("chart %q: not in the worker's chart filesystem", in.GetChart()), ErrTypeChartNotFound, err)
	}
	if err != nil {
		return nil, invalid("chart %q: %v", in.GetChart(), err)
	}
	values, err := chartutil.ReadValues([]byte(in.GetValuesYaml()))
	if err != nil {
		return nil, invalid("values: %v", err)
	}

	cfg := &action.Configuration{}
	cfg.Log = func(string, ...any) {}
	install := action.NewInstall(cfg)
	install.DryRun = true
	install.ClientOnly = true
	install.IncludeCRDs = in.GetIncludeCrds()
	install.ReleaseName = in.GetReleaseName()
	install.Namespace = in.GetNamespace()
	install.APIVersions = chartutil.VersionSet(in.GetApiVersions())
	if v := in.GetKubeVersion(); v != "" {
		kv, err := chartutil.ParseKubeVersion(v)
		if err != nil {
			return nil, invalid("kube_version: %v", err)
		}
		install.KubeVersion = kv
	}
	rel, err := install.Run(ch, values)
	if err != nil {
		return nil, invalid("render %q: %v", in.GetChart(), err)
	}
	resources, err := split(rel.Manifest, in.GetAllowSecrets())
	if err != nil {
		return nil, err
	}
	return &pb.RenderChartOutput{Resources: resources}, nil
}

// load reads one chart directory out of the filesystem.
func load(charts fs.FS, name string) (*chart.Chart, error) {
	if _, err := fs.Stat(charts, name); err != nil {
		return nil, err
	}
	sub, err := fs.Sub(charts, name)
	if err != nil {
		return nil, err
	}
	var files []*loader.BufferedFile
	err = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}
		files = append(files, &loader.BufferedFile{Name: path, Data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return loader.LoadFiles(files)
}

// split keys each rendered document by lowercase kind and name, in the order
// Helm installs them.
func split(manifest string, allowSecrets bool) ([]*kubev1.Resource, error) {
	docs := releaseutil.SplitManifests(manifest)
	keys := make([]string, 0, len(docs))
	for k := range docs {
		keys = append(keys, k)
	}
	sort.Sort(releaseutil.BySplitManifestsOrder(keys))

	var out []*kubev1.Resource
	for _, k := range keys {
		doc := docs[k]
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var head struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Metadata   struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal([]byte(doc), &head); err != nil {
			return nil, invalid("rendered document %s: %v", k, err)
		}
		if head.Kind == "" || head.Metadata.Name == "" {
			return nil, invalid("rendered document %s has no kind or metadata.name", k)
		}
		if head.Kind == "Secret" && head.APIVersion == "v1" && !allowSecrets {
			return nil, invalid("rendered Secret %q would be recorded in workflow history; set allow_secrets to carry it anyway", head.Metadata.Name)
		}
		out = append(out, &kubev1.Resource{
			Id:           strings.ToLower(head.Kind) + "-" + head.Metadata.Name,
			ManifestYaml: []byte(doc),
		})
	}
	return out, nil
}

func invalid(format string, args ...any) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf(format, args...), ErrTypeInvalid, nil)
}
