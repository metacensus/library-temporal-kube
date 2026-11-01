# Table of Contents

- [helm.v1](#helm-v1)
  - Services
    - [helm.v1.Helm](#helm-v1-helm)
      - [Activities](#helm-v1-helm-activities)
        - [helm.v1.RenderChart](#helm-v1-renderchart-activity)
  - Messages
    - [helm.v1.RenderChartInput](#helm-v1-renderchartinput)
    - [helm.v1.RenderChartOutput](#helm-v1-renderchartoutput)
- [kube.v1](#kube-v1)
  - Services
    - [kube.v1.Kubernetes](#kube-v1-kubernetes)
      - [Activities](#kube-v1-kubernetes-activities)
        - [kube.v1.Create](#kube-v1-create-activity)
        - [kube.v1.Apply](#kube-v1-apply-activity)
        - [kube.v1.Delete](#kube-v1-delete-activity)
        - [kube.v1.Status](#kube-v1-status-activity)
        - [kube.v1.Ready](#kube-v1-ready-activity)
        - [kube.v1.Gone](#kube-v1-gone-activity)
        - [kube.v1.ExecPod](#kube-v1-execpod-activity)
  - Messages
    - [kube.v1.ApplyInput](#kube-v1-applyinput)
    - [kube.v1.ApplyOutput](#kube-v1-applyoutput)
    - [kube.v1.Condition](#kube-v1-condition)
    - [kube.v1.CreateInput](#kube-v1-createinput)
    - [kube.v1.CreateOutput](#kube-v1-createoutput)
    - [kube.v1.DeleteInput](#kube-v1-deleteinput)
    - [kube.v1.DeleteOutput](#kube-v1-deleteoutput)
    - [kube.v1.ExecPodInput](#kube-v1-execpodinput)
    - [kube.v1.ExecPodOutput](#kube-v1-execpodoutput)
    - [kube.v1.GoneInput](#kube-v1-goneinput)
    - [kube.v1.GoneOutput](#kube-v1-goneoutput)
    - [kube.v1.Propagation](#kube-v1-propagation)
    - [kube.v1.ReadyInput](#kube-v1-readyinput)
    - [kube.v1.ReadyOutput](#kube-v1-readyoutput)
    - [kube.v1.Resource](#kube-v1-resource)
    - [kube.v1.ResourceIdentifier](#kube-v1-resourceidentifier)
    - [kube.v1.ResourceStatus](#kube-v1-resourcestatus)
    - [kube.v1.StatusInput](#kube-v1-statusinput)
    - [kube.v1.StatusOutput](#kube-v1-statusoutput)
- [google.protobuf](#google-protobuf)
  - Messages
    - [google.protobuf.Duration](#google-protobuf-duration)

<a name="helm-v1"></a>
# helm.v1

<a name="helm-v1-services"></a>
## Services

<a name="helm-v1-helm"></a>
## helm.v1.Helm

<pre>
Helm renders charts from the filesystem the hosting worker was built with.
Rendering touches no cluster, so the worker that holds the chart need not
hold credentials. Errors are Helm.ChartNotFound and Helm.Invalid, neither
retried: the same inputs render the same way.
</pre>   

<a name="helm-v1-helm-activities"></a>
### Activities

---
<a name="helm-v1-renderchart-activity"></a>
### helm.v1.RenderChart

<pre>
RenderChart renders one chart with the caller's values merged over the
chart's own values.yaml, validated against its values.schema.json when it
has one, and returns the resources in Helm's install order, each keyed
"<lowercase kind>-<name>". Hooks are left out. A rendered document that
does not parse fails the activity.
</pre>

**Input:** [helm.v1.RenderChartInput](#helm-v1-renderchartinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>allow_secrets</td>
<td>bool</td>
<td><pre>
A rendered kind: Secret fails with Helm.Invalid unless set: the output is
recorded in workflow history.<br>

json_name: allowSecrets
go_name: AllowSecrets</pre></td>
</tr><tr>
<td>api_versions</td>
<td>string</td>
<td><pre>
json_name: apiVersions
go_name: ApiVersions</pre></td>
</tr><tr>
<td>chart</td>
<td>string</td>
<td><pre>
Directory of the chart in the worker's chart filesystem.<br>

json_name: chart
go_name: Chart</pre></td>
</tr><tr>
<td>include_crds</td>
<td>bool</td>
<td><pre>
Include the chart's crds/ directory in the output.<br>

json_name: includeCrds
go_name: IncludeCrds</pre></td>
</tr><tr>
<td>kube_version</td>
<td>string</td>
<td><pre>
Capabilities the templates may test; defaults are Helm's.<br>

json_name: kubeVersion
go_name: KubeVersion</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
.Release.Namespace for the templates. Rendered manifests are not stamped
with it; kube's namespace field does that at write time.<br>

json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>release_name</td>
<td>string</td>
<td><pre>
Helm release name; charts derive resource names from it.<br>

json_name: releaseName
go_name: ReleaseName</pre></td>
</tr><tr>
<td>values_yaml</td>
<td>string</td>
<td><pre>
Overrides on the chart's defaults, as a values file.<br>

json_name: valuesYaml
go_name: ValuesYaml</pre></td>
</tr>
</table>

**Output:** [helm.v1.RenderChartOutput](#helm-v1-renderchartoutput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resource">kube.v1.Resource</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>start_to_close_timeout</td><td>2 minutes</td></tr>
</table>   

<a name="helm-v1-messages"></a>
## Messages

<a name="helm-v1-renderchartinput"></a>
### helm.v1.RenderChartInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>allow_secrets</td>
<td>bool</td>
<td><pre>
A rendered kind: Secret fails with Helm.Invalid unless set: the output is
recorded in workflow history.<br>

json_name: allowSecrets
go_name: AllowSecrets</pre></td>
</tr><tr>
<td>api_versions</td>
<td>string</td>
<td><pre>
json_name: apiVersions
go_name: ApiVersions</pre></td>
</tr><tr>
<td>chart</td>
<td>string</td>
<td><pre>
Directory of the chart in the worker's chart filesystem.<br>

json_name: chart
go_name: Chart</pre></td>
</tr><tr>
<td>include_crds</td>
<td>bool</td>
<td><pre>
Include the chart's crds/ directory in the output.<br>

json_name: includeCrds
go_name: IncludeCrds</pre></td>
</tr><tr>
<td>kube_version</td>
<td>string</td>
<td><pre>
Capabilities the templates may test; defaults are Helm's.<br>

json_name: kubeVersion
go_name: KubeVersion</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
.Release.Namespace for the templates. Rendered manifests are not stamped
with it; kube's namespace field does that at write time.<br>

json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>release_name</td>
<td>string</td>
<td><pre>
Helm release name; charts derive resource names from it.<br>

json_name: releaseName
go_name: ReleaseName</pre></td>
</tr><tr>
<td>values_yaml</td>
<td>string</td>
<td><pre>
Overrides on the chart's defaults, as a values file.<br>

json_name: valuesYaml
go_name: ValuesYaml</pre></td>
</tr>
</table>



<a name="helm-v1-renderchartoutput"></a>
### helm.v1.RenderChartOutput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resource">kube.v1.Resource</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1"></a>
# kube.v1

<a name="kube-v1-services"></a>
## Services

<a name="kube-v1-kubernetes"></a>
## kube.v1.Kubernetes

<pre>
Kubernetes writes pre-rendered manifests with the credentials of the worker
that hosts it. The caller keys every resource by an id of its choosing;
outputs carry the same ids, and an error names the id it stopped at.

Error types: Kube.Invalid, Kube.Forbidden, Kube.Conflict and
Kube.AlreadyExists are non-retryable; Kube.NotReady and Kube.Exists are
retryable and are how Ready and Gone wait; Kube.Failed is non-retryable
and means a resource reached a terminal failure.
</pre>   

<a name="kube-v1-kubernetes-activities"></a>
### Activities

---
<a name="kube-v1-create-activity"></a>
### kube.v1.Create

<pre>
Create posts each resource in order and fails on one that already exists,
unless it carries this activity's own idempotency annotation, in which
case a retry counts it as created.
</pre>

**Input:** [kube.v1.CreateInput](#kube-v1-createinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>allow_secrets</td>
<td>bool</td>
<td><pre>
A kind: Secret fails with Kube.Invalid unless set: a manifest is recorded
in workflow history.<br>

json_name: allowSecrets
go_name: AllowSecrets</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
Stamped on a namespaced resource whose manifest names none. A namespaced
resource with neither fails with Kube.Invalid.<br>

json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>resources</td>
<td><a href="#kube-v1-resource">kube.v1.Resource</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Output:** [kube.v1.CreateOutput](#kube-v1-createoutput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>start_to_close_timeout</td><td>2 minutes</td></tr>
</table> 

---
<a name="kube-v1-apply-activity"></a>
### kube.v1.Apply

<pre>
Apply is server-side apply under the worker's field manager, in order.
A field another manager owns fails with Kube.Conflict unless force is set.
</pre>

**Input:** [kube.v1.ApplyInput](#kube-v1-applyinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>allow_secrets</td>
<td>bool</td>
<td><pre>
json_name: allowSecrets
go_name: AllowSecrets</pre></td>
</tr><tr>
<td>force</td>
<td>bool</td>
<td><pre>
Take ownership of fields another manager owns instead of failing.<br>

json_name: force
go_name: Force</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>resources</td>
<td><a href="#kube-v1-resource">kube.v1.Resource</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Output:** [kube.v1.ApplyOutput](#kube-v1-applyoutput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>start_to_close_timeout</td><td>2 minutes</td></tr>
</table> 

---
<a name="kube-v1-delete-activity"></a>
### kube.v1.Delete

<pre>
Delete removes each resource. Absent is success. An identifier that
carries a uid deletes only that object; a different object under the
same name is left alone and counts as gone.
</pre>

**Input:** [kube.v1.DeleteInput](#kube-v1-deleteinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>propagation</td>
<td><a href="#kube-v1-propagation">kube.v1.Propagation</a></td>
<td><pre>
json_name: propagation
go_name: Propagation</pre></td>
</tr><tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Output:** [kube.v1.DeleteOutput](#kube-v1-deleteoutput)



**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>start_to_close_timeout</td><td>2 minutes</td></tr>
</table> 

---
<a name="kube-v1-status-activity"></a>
### kube.v1.Status

<pre>
Status reports each resource's kstatus condition and never fails because
a resource is not ready.
</pre>

**Input:** [kube.v1.StatusInput](#kube-v1-statusinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Output:** [kube.v1.StatusOutput](#kube-v1-statusoutput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourcestatus">kube.v1.ResourceStatus</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>start_to_close_timeout</td><td>1 minute</td></tr>
</table> 

---
<a name="kube-v1-ready-activity"></a>
### kube.v1.Ready

<pre>
Ready fails with Kube.NotReady while any resource is not Current, so the
caller's retry policy is the wait and its schedule-to-close timeout the
cap. A resource in Failed fails with Kube.Failed, which is not retried.
</pre>

**Input:** [kube.v1.ReadyInput](#kube-v1-readyinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Output:** [kube.v1.ReadyOutput](#kube-v1-readyoutput)



**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>retry_policy.backoff_coefficient</td><td>1</td></tr>
<tr><td>retry_policy.initial_interval</td><td>5 seconds</td></tr>
<tr><td>start_to_close_timeout</td><td>1 minute</td></tr>
</table> 

---
<a name="kube-v1-gone-activity"></a>
### kube.v1.Gone

<pre>
Gone fails with Kube.Exists while any resource still exists, with the
same wait contract as Ready.
</pre>

**Input:** [kube.v1.GoneInput](#kube-v1-goneinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>

**Output:** [kube.v1.GoneOutput](#kube-v1-goneoutput)



**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>retry_policy.backoff_coefficient</td><td>1</td></tr>
<tr><td>retry_policy.initial_interval</td><td>5 seconds</td></tr>
<tr><td>start_to_close_timeout</td><td>1 minute</td></tr>
</table> 

---
<a name="kube-v1-execpod-activity"></a>
### kube.v1.ExecPod

<pre>
ExecPod runs a command in a container and returns what it wrote. A
non-zero exit is a result, not an error; the activity fails only when
the exec itself cannot run. It runs once, because a command is not
idempotent; callers opt into retries.
</pre>

**Input:** [kube.v1.ExecPodInput](#kube-v1-execpodinput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>command</td>
<td>string</td>
<td><pre>
json_name: command
go_name: Command</pre></td>
</tr><tr>
<td>container</td>
<td>string</td>
<td><pre>
json_name: container
go_name: Container</pre></td>
</tr><tr>
<td>max_output_bytes</td>
<td>int64</td>
<td><pre>
Cap on stdout and on stderr, each; beyond it the output is cut and
truncated is set. Zero means 512 KiB, under Temporal's payload limit.<br>

json_name: maxOutputBytes
go_name: MaxOutputBytes</pre></td>
</tr><tr>
<td>pod</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
The pod, by namespace and name; api_version and kind may be empty.<br>

json_name: pod
go_name: Pod</pre></td>
</tr><tr>
<td>stdin</td>
<td>bytes</td>
<td><pre>
json_name: stdin
go_name: Stdin</pre></td>
</tr><tr>
<td>timeout</td>
<td><a href="#google-protobuf-duration">google.protobuf.Duration</a></td>
<td><pre>
Deadline for the command; zero means the activity's own timeout.<br>

json_name: timeout
go_name: Timeout</pre></td>
</tr>
</table>

**Output:** [kube.v1.ExecPodOutput](#kube-v1-execpodoutput)

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>exit_code</td>
<td>int32</td>
<td><pre>
json_name: exitCode
go_name: ExitCode</pre></td>
</tr><tr>
<td>stderr</td>
<td>bytes</td>
<td><pre>
json_name: stderr
go_name: Stderr</pre></td>
</tr><tr>
<td>stdout</td>
<td>bytes</td>
<td><pre>
json_name: stdout
go_name: Stdout</pre></td>
</tr><tr>
<td>truncated</td>
<td>bool</td>
<td><pre>
json_name: truncated
go_name: Truncated</pre></td>
</tr>
</table>

**Defaults:**

<table>
<tr><th>Name</th><th>Value</th></tr>
<tr><td>heartbeat_timeout</td><td>30 seconds</td></tr>
<tr><td>retry_policy.max_attempts</td><td>1</td></tr>
<tr><td>schedule_to_close_timeout</td><td>30 seconds</td></tr>
<tr><td>start_to_close_timeout</td><td>5 minutes</td></tr>
</table>   

<a name="kube-v1-messages"></a>
## Messages

<a name="kube-v1-applyinput"></a>
### kube.v1.ApplyInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>allow_secrets</td>
<td>bool</td>
<td><pre>
json_name: allowSecrets
go_name: AllowSecrets</pre></td>
</tr><tr>
<td>force</td>
<td>bool</td>
<td><pre>
Take ownership of fields another manager owns instead of failing.<br>

json_name: force
go_name: Force</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>resources</td>
<td><a href="#kube-v1-resource">kube.v1.Resource</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-applyoutput"></a>
### kube.v1.ApplyOutput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-condition"></a>
### kube.v1.Condition

<pre>
Condition is kstatus's reading of an object: Current means reconciled to
the generation that was written, InProgress means still reconciling,
Failed means it will not reconcile without a change. A Pod that exited
with failure is Failed here, where kstatus would call it Current.
</pre>

<table>
<tr><th>Value</th><th>Description</th></tr>
<tr>
<td>CONDITION_UNSPECIFIED</td>
<td></td>
</tr><tr>
<td>CONDITION_CURRENT</td>
<td></td>
</tr><tr>
<td>CONDITION_IN_PROGRESS</td>
<td></td>
</tr><tr>
<td>CONDITION_FAILED</td>
<td></td>
</tr><tr>
<td>CONDITION_TERMINATING</td>
<td></td>
</tr><tr>
<td>CONDITION_NOT_FOUND</td>
<td></td>
</tr>
</table>

<a name="kube-v1-createinput"></a>
### kube.v1.CreateInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>allow_secrets</td>
<td>bool</td>
<td><pre>
A kind: Secret fails with Kube.Invalid unless set: a manifest is recorded
in workflow history.<br>

json_name: allowSecrets
go_name: AllowSecrets</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
Stamped on a namespaced resource whose manifest names none. A namespaced
resource with neither fails with Kube.Invalid.<br>

json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>resources</td>
<td><a href="#kube-v1-resource">kube.v1.Resource</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-createoutput"></a>
### kube.v1.CreateOutput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-deleteinput"></a>
### kube.v1.DeleteInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>propagation</td>
<td><a href="#kube-v1-propagation">kube.v1.Propagation</a></td>
<td><pre>
json_name: propagation
go_name: Propagation</pre></td>
</tr><tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-deleteoutput"></a>
### kube.v1.DeleteOutput



<a name="kube-v1-execpodinput"></a>
### kube.v1.ExecPodInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>command</td>
<td>string</td>
<td><pre>
json_name: command
go_name: Command</pre></td>
</tr><tr>
<td>container</td>
<td>string</td>
<td><pre>
json_name: container
go_name: Container</pre></td>
</tr><tr>
<td>max_output_bytes</td>
<td>int64</td>
<td><pre>
Cap on stdout and on stderr, each; beyond it the output is cut and
truncated is set. Zero means 512 KiB, under Temporal's payload limit.<br>

json_name: maxOutputBytes
go_name: MaxOutputBytes</pre></td>
</tr><tr>
<td>pod</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
The pod, by namespace and name; api_version and kind may be empty.<br>

json_name: pod
go_name: Pod</pre></td>
</tr><tr>
<td>stdin</td>
<td>bytes</td>
<td><pre>
json_name: stdin
go_name: Stdin</pre></td>
</tr><tr>
<td>timeout</td>
<td><a href="#google-protobuf-duration">google.protobuf.Duration</a></td>
<td><pre>
Deadline for the command; zero means the activity's own timeout.<br>

json_name: timeout
go_name: Timeout</pre></td>
</tr>
</table>



<a name="kube-v1-execpodoutput"></a>
### kube.v1.ExecPodOutput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>exit_code</td>
<td>int32</td>
<td><pre>
json_name: exitCode
go_name: ExitCode</pre></td>
</tr><tr>
<td>stderr</td>
<td>bytes</td>
<td><pre>
json_name: stderr
go_name: Stderr</pre></td>
</tr><tr>
<td>stdout</td>
<td>bytes</td>
<td><pre>
json_name: stdout
go_name: Stdout</pre></td>
</tr><tr>
<td>truncated</td>
<td>bool</td>
<td><pre>
json_name: truncated
go_name: Truncated</pre></td>
</tr>
</table>



<a name="kube-v1-goneinput"></a>
### kube.v1.GoneInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-goneoutput"></a>
### kube.v1.GoneOutput



<a name="kube-v1-propagation"></a>
### kube.v1.Propagation

<pre>
Propagation is the Kubernetes deletion propagation policy.
</pre>

<table>
<tr><th>Value</th><th>Description</th></tr>
<tr>
<td>PROPAGATION_UNSPECIFIED</td>
<td><pre>
Background: the object goes first, its dependents after.
</pre></td>
</tr><tr>
<td>PROPAGATION_BACKGROUND</td>
<td></td>
</tr><tr>
<td>PROPAGATION_FOREGROUND</td>
<td></td>
</tr><tr>
<td>PROPAGATION_ORPHAN</td>
<td></td>
</tr>
</table>

<a name="kube-v1-readyinput"></a>
### kube.v1.ReadyInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-readyoutput"></a>
### kube.v1.ReadyOutput



<a name="kube-v1-resource"></a>
### kube.v1.Resource

<pre>
Resource is one manifest under the caller's id.
</pre>

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>id</td>
<td>string</td>
<td><pre>
json_name: id
go_name: Id</pre></td>
</tr><tr>
<td>manifest_yaml</td>
<td>bytes</td>
<td><pre>
json_name: manifestYaml
go_name: ManifestYaml</pre></td>
</tr>
</table>



<a name="kube-v1-resourceidentifier"></a>
### kube.v1.ResourceIdentifier

<pre>
ResourceIdentifier names one object in the cluster under the caller's id.
Writes return it with uid and generation set; callers that already know
the object may build it with only api_version, kind, name and namespace.
</pre>

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>api_version</td>
<td>string</td>
<td><pre>
json_name: apiVersion
go_name: ApiVersion</pre></td>
</tr><tr>
<td>generation</td>
<td>int64</td>
<td><pre>
json_name: generation
go_name: Generation</pre></td>
</tr><tr>
<td>id</td>
<td>string</td>
<td><pre>
json_name: id
go_name: Id</pre></td>
</tr><tr>
<td>kind</td>
<td>string</td>
<td><pre>
json_name: kind
go_name: Kind</pre></td>
</tr><tr>
<td>name</td>
<td>string</td>
<td><pre>
json_name: name
go_name: Name</pre></td>
</tr><tr>
<td>namespace</td>
<td>string</td>
<td><pre>
Empty for a cluster-scoped resource.<br>

json_name: namespace
go_name: Namespace</pre></td>
</tr><tr>
<td>uid</td>
<td>string</td>
<td><pre>
json_name: uid
go_name: Uid</pre></td>
</tr>
</table>



<a name="kube-v1-resourcestatus"></a>
### kube.v1.ResourceStatus

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>condition</td>
<td><a href="#kube-v1-condition">kube.v1.Condition</a></td>
<td><pre>
json_name: condition
go_name: Condition</pre></td>
</tr><tr>
<td>generation</td>
<td>int64</td>
<td><pre>
json_name: generation
go_name: Generation</pre></td>
</tr><tr>
<td>id</td>
<td>string</td>
<td><pre>
json_name: id
go_name: Id</pre></td>
</tr><tr>
<td>message</td>
<td>string</td>
<td><pre>
json_name: message
go_name: Message</pre></td>
</tr>
</table>



<a name="kube-v1-statusinput"></a>
### kube.v1.StatusInput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourceidentifier">kube.v1.ResourceIdentifier</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>



<a name="kube-v1-statusoutput"></a>
### kube.v1.StatusOutput

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>resources</td>
<td><a href="#kube-v1-resourcestatus">kube.v1.ResourceStatus</a></td>
<td><pre>
json_name: resources
go_name: Resources</pre></td>
</tr>
</table>




<a name="google-protobuf"></a>
# google.protobuf

<a name="google-protobuf-messages"></a>
## Messages

<a name="google-protobuf-duration"></a>
### google.protobuf.Duration

<pre>
A Duration represents a signed, fixed-length span of time represented
as a count of seconds and fractions of seconds at nanosecond
resolution. It is independent of any calendar and concepts like "day"
or "month". It is related to Timestamp in that the difference between
two Timestamp values is a Duration and it can be added or subtracted
from a Timestamp. Range is approximately +-10,000 years.

# Examples

Example 1: Compute Duration from two Timestamps in pseudo code.

    Timestamp start = ...;
    Timestamp end = ...;
    Duration duration = ...;

    duration.seconds = end.seconds - start.seconds;
    duration.nanos = end.nanos - start.nanos;

    if (duration.seconds < 0 && duration.nanos > 0) {
      duration.seconds += 1;
      duration.nanos -= 1000000000;
    } else if (duration.seconds > 0 && duration.nanos < 0) {
      duration.seconds -= 1;
      duration.nanos += 1000000000;
    }

Example 2: Compute Timestamp from Timestamp + Duration in pseudo code.

    Timestamp start = ...;
    Duration duration = ...;
    Timestamp end = ...;

    end.seconds = start.seconds + duration.seconds;
    end.nanos = start.nanos + duration.nanos;

    if (end.nanos < 0) {
      end.seconds -= 1;
      end.nanos += 1000000000;
    } else if (end.nanos >= 1000000000) {
      end.seconds += 1;
      end.nanos -= 1000000000;
    }

Example 3: Compute Duration from datetime.timedelta in Python.

    td = datetime.timedelta(days=3, minutes=10)
    duration = Duration()
    duration.FromTimedelta(td)

# JSON Mapping

In JSON format, the Duration type is encoded as a string rather than an
object, where the string ends in the suffix "s" (indicating seconds) and
is preceded by the number of seconds, with nanoseconds expressed as
fractional seconds. For example, 3 seconds with 0 nanoseconds should be
encoded in JSON format as "3s", while 3 seconds and 1 nanosecond should
be expressed in JSON format as "3.000000001s", and 3 seconds and 1
microsecond should be expressed in JSON format as "3.000001s".
</pre>

<table>
<tr>
<th>Attribute</th>
<th>Type</th>
<th>Description</th>
</tr>
<tr>
<td>nanos</td>
<td>int32</td>
<td><pre>
Signed fractions of a second at nanosecond resolution of the span
of time. Durations less than one second are represented with a 0
`seconds` field and a positive or negative `nanos` field. For durations
of one second or more, a non-zero value for the `nanos` field must be
of the same sign as the `seconds` field. Must be from -999,999,999
to +999,999,999 inclusive.<br>

json_name: nanos
go_name: Nanos</pre></td>
</tr><tr>
<td>seconds</td>
<td>int64</td>
<td><pre>
Signed seconds of the span of time. Must be from -315,576,000,000
to +315,576,000,000 inclusive. Note: these bounds are computed from:
60 sec/min * 60 min/hr * 24 hr/day * 365.25 days/year * 10000 years<br>

json_name: seconds
go_name: Seconds</pre></td>
</tr>
</table>

