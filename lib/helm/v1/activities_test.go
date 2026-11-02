package helm_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"go.temporal.io/sdk/temporal"

	pb "github.com/metacensus/library-temporal-kube/gen/helm/v1"
	helm "github.com/metacensus/library-temporal-kube/lib/helm/v1"
)

// charts is one chart, "basic", with defaults, a schema, a conditional
// Secret, a hook and a CRD, so each rendering rule has something to act on.
var charts = fstest.MapFS{
	"basic/Chart.yaml": {Data: []byte("apiVersion: v2\nname: basic\nversion: 0.1.0\n")},
	"basic/values.yaml": {Data: []byte(`name: app
replicas: 1
port: 80
secret:
  enabled: false
`)},
	"basic/values.schema.json": {Data: []byte(`{"type":"object","properties":{"replicas":{"type":"integer"},"name":{"type":"string"}}}`)},
	"basic/templates/namespace.yaml": {Data: []byte(`apiVersion: v1
kind: Namespace
metadata:
  name: {{ .Values.name }}-ns
`)},
	"basic/templates/service.yaml": {Data: []byte(`apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-{{ .Chart.Name }}
  labels:
    release-namespace: {{ .Release.Namespace | quote }}
  {{- if semverCompare ">=1.99-0" .Capabilities.KubeVersion.Version }}
  annotations:
    future: "true"
  {{- end }}
spec:
  ports:
  - port: {{ .Values.port }}
`)},
	"basic/templates/deployment.yaml": {Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-{{ .Chart.Name }}
spec:
  replicas: {{ .Values.replicas }}
`)},
	"basic/templates/secret.yaml": {Data: []byte(`{{- if .Values.secret.enabled }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Release.Name }}-creds
stringData:
  password: hunter2
{{- end }}
`)},
	"basic/templates/hook.yaml": {Data: []byte(`apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Release.Name }}-migrate
  annotations:
    "helm.sh/hook": pre-install
`)},
	"basic/crds/widgets.yaml": {Data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.acme.io
`)},
}

func assertErrType(t *testing.T, err error, errType string) {
	t.Helper()
	if errType == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) || appErr.Type() != errType || !appErr.NonRetryable() {
		t.Fatalf("want non-retryable %s, got %T: %v", errType, err, err)
	}
}

func ids(out *pb.RenderChartOutput) []string {
	var got []string
	for _, r := range out.GetResources() {
		got = append(got, r.GetId())
	}
	return got
}

func find(out *pb.RenderChartOutput, id string) string {
	for _, r := range out.GetResources() {
		if r.GetId() == id {
			return string(r.GetManifestYaml())
		}
	}
	return ""
}

func TestActivities_RenderChart(t *testing.T) {
	a := helm.NewActivities(charts)
	base := func() *pb.RenderChartInput {
		return &pb.RenderChartInput{Chart: "basic", ReleaseName: "rel", Namespace: "team"}
	}
	tests := []struct {
		name    string
		in      *pb.RenderChartInput
		wantIDs []string
		check   func(t *testing.T, out *pb.RenderChartOutput)
		errType string
	}{
		{
			name:    "error - chart is required",
			in:      &pb.RenderChartInput{ReleaseName: "rel"},
			errType: helm.ErrTypeInvalid,
		},
		{
			name:    "error - release_name is required",
			in:      &pb.RenderChartInput{Chart: "basic"},
			errType: helm.ErrTypeInvalid,
		},
		{
			name:    "error - chart not in the filesystem",
			in:      &pb.RenderChartInput{Chart: "missing", ReleaseName: "rel"},
			errType: helm.ErrTypeChartNotFound,
		},
		{
			name: "error - values are not YAML",
			in: func() *pb.RenderChartInput {
				in := base()
				in.ValuesYaml = "port: [80"
				return in
			}(),
			errType: helm.ErrTypeInvalid,
		},
		{
			name: "error - values fail the chart's schema",
			in: func() *pb.RenderChartInput {
				in := base()
				in.ValuesYaml = "replicas: two\n"
				return in
			}(),
			errType: helm.ErrTypeInvalid,
		},
		{
			name:    "success - install order, ids by kind and name, hooks left out",
			in:      base(),
			wantIDs: []string{"namespace-app-ns", "service-rel-basic", "deployment-rel-basic"},
		},
		{
			name: "success - overrides merge over the chart's defaults",
			in: func() *pb.RenderChartInput {
				in := base()
				in.ValuesYaml = "port: 8080\n"
				return in
			}(),
			wantIDs: []string{"namespace-app-ns", "service-rel-basic", "deployment-rel-basic"},
			check: func(t *testing.T, out *pb.RenderChartOutput) {
				if !strings.Contains(find(out, "service-rel-basic"), "port: 8080") {
					t.Fatalf("override did not reach the Service:\n%s", find(out, "service-rel-basic"))
				}
				if !strings.Contains(find(out, "deployment-rel-basic"), "replicas: 1") {
					t.Fatalf("default did not reach the Deployment:\n%s", find(out, "deployment-rel-basic"))
				}
			},
		},
		{
			name: "success - the release namespace reaches the templates",
			in:   base(),
			check: func(t *testing.T, out *pb.RenderChartOutput) {
				if !strings.Contains(find(out, "service-rel-basic"), `release-namespace: "team"`) {
					t.Fatalf("namespace missing from template context:\n%s", find(out, "service-rel-basic"))
				}
			},
		},
		{
			name: "success - kube_version drives capabilities",
			in: func() *pb.RenderChartInput {
				in := base()
				in.KubeVersion = "v1.99.0"
				return in
			}(),
			check: func(t *testing.T, out *pb.RenderChartOutput) {
				if !strings.Contains(find(out, "service-rel-basic"), `future: "true"`) {
					t.Fatalf("capability gate did not open:\n%s", find(out, "service-rel-basic"))
				}
				if strings.Contains(find(renderDefault(t, a), "service-rel-basic"), "future") {
					t.Fatal("capability gate should stay closed by default")
				}
			},
		},
		{
			name: "success - include_crds puts the CRD first",
			in: func() *pb.RenderChartInput {
				in := base()
				in.IncludeCrds = true
				return in
			}(),
			wantIDs: []string{"customresourcedefinition-widgets.acme.io", "namespace-app-ns", "service-rel-basic", "deployment-rel-basic"},
		},
		{
			name: "error - a rendered Secret is refused unless allowed",
			in: func() *pb.RenderChartInput {
				in := base()
				in.ValuesYaml = "secret:\n  enabled: true\n"
				return in
			}(),
			errType: helm.ErrTypeInvalid,
		},
		{
			name: "success - a rendered Secret passes when allowed",
			in: func() *pb.RenderChartInput {
				in := base()
				in.ValuesYaml = "secret:\n  enabled: true\n"
				in.AllowSecrets = true
				return in
			}(),
			wantIDs: []string{"namespace-app-ns", "secret-rel-creds", "service-rel-basic", "deployment-rel-basic"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := a.RenderChart(context.Background(), tt.in)
			assertErrType(t, err, tt.errType)
			if tt.errType != "" {
				return
			}
			if tt.wantIDs != nil && strings.Join(ids(out), ",") != strings.Join(tt.wantIDs, ",") {
				t.Fatalf("want ids %v, got %v", tt.wantIDs, ids(out))
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func renderDefault(t *testing.T, a *helm.Activities) *pb.RenderChartOutput {
	t.Helper()
	out, err := a.RenderChart(context.Background(), &pb.RenderChartInput{Chart: "basic", ReleaseName: "rel"})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
