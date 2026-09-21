# testctl

`testctl` is a CLI tool that runs a sequence of test actions against a Cluster API management
cluster, driven by a YAML configuration file. It is built around a small, extensible
plugin system: each step in a config file invokes a named *plugin* that either **selects**
a set of objects to act on (e.g. Clusters, MachineDeployments) or **executes** an action on
them (e.g. scale, delete, upgrade).

- [Usage](#usage)
- [Config file format](#config-file-format)
  - [Top-level shape](#top-level-shape)
  - [`runConfig`](#runconfig)
  - [Importing another config file](#importing-another-config-file)
- [Plugin reference](#plugin-reference)
- [Writing a new plugin](#writing-a-new-plugin)
- [Known limitations](#known-limitations)

## Usage

```shell
go run ./hack/tools/testctl \
  --kubeconfig ~/.kube/config \
  --kubeconfig-context my-management-cluster \
  --config ./my-test.yaml
```

Flags:

| Flag | Description |
|---|---|
| `--kubeconfig` | Path to the kubeconfig file for the management cluster. If empty, default discovery rules apply. |
| `--kubeconfig-context` | Context to use within the kubeconfig file. If empty, the current context is used. |
| `--config`, `-c` | Path to the YAML file describing the test sequence to run (required). |

`testctl` also exposes the standard `k8s.io/component-base/logs` flags (e.g. `--v` for log
verbosity).

## Config file format

### Top-level shape

The whole config file is a single YAML document describing what to run. At the top level it
must have a `run` list, and may optionally have a `runConfig`:

```yaml
run:
  - plugin: select.cluster.testctl
    nameRegex: "^my-cluster-"
    run:
      - plugin: edit.topology.cluster.testctl
        version: v1.37.0
    runConfig:
      concurrency: 2
runConfig:
  dryRun: false
```

Every entry in a `run` (or `runOnError`/`runAfter`, see below) list is a map with:

- `plugin` (required): the key of the plugin to invoke, e.g. `select.cluster.testctl`. See
  [Plugin reference](#plugin-reference) for the full list.
- any additional fields the plugin accepts as its own config (see the plugin's config type in
  the reference below). These are parsed strictly — an unknown field is a configuration error.
- `run`: a nested list of plugin invocations to run against every object selected by this
  entry. **Required** if the plugin is a selector plugin (it defines what happens with the
  selected objects), and not allowed for a plugin that only executes an action.
- `runOnError` / `runAfter` (both optional): lists of plugin invocations, with the same shape
  as `run`, executed after this entry's own action (and, for selectors, after its nested `run`
  list) completes. `runOnError` only runs if this entry (or something under it) failed;
  `runAfter` always runs, similar to a `defer`. Both are useful for cleanup steps.
- `runConfig` (optional): overrides for this entry and everything nested under it, see below.

### `runConfig`

`runConfig` can be set at the top level and/or on any individual `run`/`runOnError`/`runAfter`
entry. Fields:

| Field | Type | Default     | Description |
|---|---|-------------|---|
| `limit` | int | unlimited   | Max number of selected objects to consider for a selector plugin, or objects to create for `create.machineDeployment.topology.cluster.testctl`. |
| `concurrency` | int | `1`         | Number of objects to process in parallel when a selector plugin returns more than one object. |
| `failFast` | bool | `false`     | If `true`, stop scheduling new work as soon as one parallel action fails (already in-flight actions are still allowed to complete). |
| `dryRun` | bool | `false` (*) | Run in dry-run mode: plugins log what they would do but don't mutate the cluster. Inherited from the top-level `runConfig` if not set locally. |
| `skip` | bool | `false` | Skip this plugin, including its nested `run`, `runOnError`, `runAfter` lists and debug prompts. At the top level, skips the entire run. Configuration is still validated. |
| `skipWait` | bool | `false`     | Don't wait for an action to converge (e.g. for a scale-up to complete) before moving on. |
| `debug` | bool | `false` (*)     | Prompt for confirmation before running each plugin. Inherited from the top-level `runConfig` if not set locally. |
| `debugOnError` | bool | `false` (*)     | Prompt for confirmation after a plugin fails. Inherited from the top-level `runConfig` if not set locally. |
| `timeout` | duration | none        | Timeout applied to the plugin call (e.g. `5m`). |
| `intervals` | map of duration lists | none        | Reserved for future use — merged with the top-level `runConfig`'s map, but not currently read by any of the shipped plugins. |

(*): if a value for `dryRun`, `debug`, `debugOnError` is provided in the top level `runConfig`, it is used as a default for the `runConfig`
of the plugins calls at lower levels.

For example, temporarily disable an action with `runConfig.skip`:

```yaml
run:
  - plugin: apply.manifest.testctl
    path: ./manifest.yaml
    runConfig:
      skip: true
```

### Importing another config file

The built-in `import.config.testctl` plugin lets you split a config across multiple files. Use
it in place of a regular plugin entry, anywhere a plugin entry is valid — at the top level, or
nested inside a `run`, `runOnError`, or `runAfter` list at any depth:

```yaml
run:
  - plugin: import.config.testctl
    path: ./common/scale-up.yaml
```

The referenced file's content replaces this entry entirely (its `plugin` field is dropped and
the rest of the entry is merged in). Rules and gotchas:

- The importing entry's own config **cannot** have a `run` field — that must come from the
  imported file.
- If the importing entry defines its own `runConfig`, it **overrides** the one from the
  imported file (rather than being merged with it).

## Plugin reference

Every plugin is registered under a stable key of the form `<verb>.<noun>.testctl`. Selector
plugins add an object to the shared `TestObjects` map (keyed as noted below) so that nested
plugins can act on it; most non-selector plugins require a specific selector as an ancestor in
the call stack (see "Requires").

| Key | Kind | Sets | Requires |
|---|---|---|---|
| `select.cluster.testctl` | selector | `Cluster` | — |
| `delete.cluster.testctl` | executor | — | `select.cluster.testctl` |
| `edit.topology.cluster.testctl` | executor | — | `select.cluster.testctl` |
| `check.topology.cluster.testctl` | executor | — | `select.cluster.testctl` |
| `update.controlPlaneEndpoint.cluster.testctl` | executor | — | `select.cluster.testctl` |
| `select.controlPlane.testctl` | selector | `ControlPlane` | `select.cluster.testctl` |
| `edit.controlPlane.topology.cluster.testctl` | executor | — | `select.controlPlane.testctl` |
| `select.machineDeployment.testctl` | selector | `MachineDeployment` | `select.cluster.testctl` |
| `create.machineDeployment.topology.cluster.testctl` | executor | — | `select.cluster.testctl` |
| `edit.machineDeployment.topology.cluster.testctl` | executor | — | `select.machineDeployment.testctl` |
| `delete.machineDeployment.topology.cluster.testctl` | executor | — | `select.machineDeployment.testctl` |
| `install.helmChart.testctl` | executor | — | none (target is auto-detected, see below) |
| `uninstall.helmChart.testctl` | executor | — | none (target is auto-detected, see below) |
| `apply.manifest.testctl` | executor | — | none (target is auto-detected, see below) |

### `select.cluster.testctl`

Go type: `plugins.ClusterSelectorPlugin`, config: `plugins.ClusterSelectorPluginConfig`.

Selects the Clusters to act on. This is the entry point for every cluster-scoped chain of
plugins and has no call-stack requirement of its own.

```yaml
- plugin: select.cluster.testctl
  clusterSelectors:      # optional, list of label selectors; results across selectors are ORed
    - matchLabels:
        environment: test
  nameRegex: "^my-cluster-"  # optional, further filters candidates by Cluster name
  run: [...]
```

### `delete.cluster.testctl`

Go type: `plugins.ClusterDeletePlugin` (no config).

Deletes the selected Cluster and waits for it to be removed (unless `skipWait` is set).

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: delete.cluster.testctl
```

### `edit.topology.cluster.testctl`

Go type: `plugins.TopologyEditPlugin`, config: `plugins.TopologyEditPluginConfig`.

Patches any subset of `spec.topology` fields on the selected Cluster in a single patch (so
multiple changes, e.g. upgrading the version and rebasing to a new ClusterClass, are applied
concurrently) and waits for the control plane and all MachineDeployments to converge. Requires
the Cluster to use ClusterClass (`spec.topology` must be defined).

`spec.topology.controlPlane` and `spec.topology.workers.machineDeployments` fields are out of
scope for this plugin — they're handled by
[`edit.controlPlane.topology.cluster.testctl`](#editcontrolplanetopologyclustertestctl) and
[`edit.machineDeployment.topology.cluster.testctl`](#editmachinedeploymenttopologyclustertestctl).

Every config field is optional; only the fields you set are patched:

| Field | Patches | Extra wait behavior |
|---|---|---|
| `version` | `spec.topology.version` | waits for the control plane and all MachineDeployments to roll out to the new version |
| `classRef` | `spec.topology.classRef` | waits for the control plane and all MachineDeployments to converge against the new ClusterClass |
| `variables` | `spec.topology.variables` | — (their impact is defined by patches, so no direct convergence signal exists) |

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: edit.topology.cluster.testctl
      version: v1.31.0
```

### `check.topology.cluster.testctl`

Go type: `plugins.TopologyCheckPlugin`, config: `plugins.TopologyCheckPluginConfig`.

Read-only: checks that the selected Cluster's topology is in the desired state, without
patching anything. Requires ClusterClass (`spec.topology` must be defined).

`checks` selects which of the following to perform; if omitted, every check runs:

| Check | What it verifies |
|---|---|
| `Availability` | the Cluster's own `Available` condition is `True` |
| `ControlPlane` | the same checks performed by [`edit.controlPlane.topology.cluster.testctl`](#editcontrolplanetopologyclustertestctl) (`waitForControlPlane`, `waitForControlPlaneMachineHealthCheck`, `waitForControlPlaneMachines`) |
| `MachineDeployments` | the same checks performed by [`edit.machineDeployment.topology.cluster.testctl`](#editmachinedeploymenttopologyclustertestctl) (`waitForMachineDeployment`, `waitForMachineDeploymentMachineHealthCheck`, `waitForMachineDeploymentMachines`) for every MachineDeployment |

Like the edit plugins, it retries every 5s until the selected checks pass or `timeout` elapses
(default 5m); set `skipWait: true` to check once and fail immediately if not currently
satisfied. `dryRun` has no effect, since this plugin never mutates anything.

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: check.topology.cluster.testctl
      checks: [Availability, ControlPlane, MachineDeployments]   # optional, defaults to all three
```

### `update.controlPlaneEndpoint.cluster.testctl`

Go type: `plugins.ClusterControlPlaneEndpointPlugin`, config:
`plugins.ClusterControlPlaneEndpointPluginConfig`.

Starts or stops the in-memory control plane endpoint listener for an in-memory-backed
Cluster, by calling a local HTTP endpoint (`http://127.0.0.1:19000`). 

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: update.controlPlaneEndpoint.cluster.testctl
      status: NotRunning   # "Running" (default) or "NotRunning"
```

### `select.controlPlane.testctl`

Go type: `plugins.ControlPlaneSelectorPlugin` (no config).

Selects the Cluster's control plane. Only `KubeadmControlPlane` is currently supported.

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: select.controlPlane.testctl
      run: [...]
```

### `edit.controlPlane.topology.cluster.testctl`

Go type: `plugins.ControlPlaneEditPlugin`, config: `plugins.ControlPlaneEditPluginConfig`.

Patches any subset of `spec.topology.controlPlane` fields on the Cluster in a single patch
(so multiple changes, e.g. scaling up and changing taints, are applied concurrently) and waits
for the change to converge. Requires ClusterClass (`spec.topology` must be defined).

Every config field is optional; a nil field means "don't touch"; a non-nil field means "set the
corresponding ControlPlaneTopology field to exactly this value":

| Field | Patches | Extra wait behavior |
|---|---|---|
| `metadata` | `spec.topology.controlPlane.metadata` | waits for the labels/annotations to appear on the KubeadmControlPlane |
| `replicas` | `spec.topology.controlPlane.replicas` | waits for the KubeadmControlPlane to reach the target replica count |
| `rollout` | `spec.topology.controlPlane.rollout` | — |
| `healthCheck` | `spec.topology.controlPlane.healthCheck` | waits for the corresponding MachineHealthCheck to reflect the desired checks/remediation (or to be absent, if disabled) |
| `deletion` | `spec.topology.controlPlane.deletion` | — |
| `taints` | `spec.topology.controlPlane.taints` | waits for every ControlPlane Machine's `spec.taints` to match |
| `readinessGates` | `spec.topology.controlPlane.readinessGates` | — |
| `variables` | `spec.topology.controlPlane.variables` | — |

Regardless of which fields are set, the plugin always waits for every ControlPlane Machine to
have the `Available` condition `True`, and for the KubeadmControlPlane itself to be `Available`
and not `RollingOut` (i.e. not updating in place) before returning.

```yaml
- plugin: select.controlPlane.testctl
  run:
    - plugin: edit.controlPlane.topology.cluster.testctl
      replicas: 3
      taints:
        - key: node-role.kubernetes.io/control-plane
          effect: NoSchedule
          propagation: Always
```

### `select.machineDeployment.testctl`

Go type: `plugins.MachineDeploymentSelectorPlugin`, config:
`plugins.MachineDeploymentSelectorPluginConfig`.

Selects the Cluster's MachineDeployments.

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: select.machineDeployment.testctl
      topologyNameRegex: "^workers-"   # optional, filters by topology name
      run: [...]
```

### `create.machineDeployment.topology.cluster.testctl`

Go type: `plugins.MachineDeploymentCreatePlugin`, config:
`plugins.MachineDeploymentCreatePluginConfig`.

Appends one or more MachineDeployment topologies to `spec.topology.workers.machineDeployments`
and waits for the resulting MachineDeployments to come up. Requires ClusterClass
(`spec.topology` must be defined).

```yaml
- plugin: select.cluster.testctl
  run:
    - plugin: create.machineDeployment.topology.cluster.testctl
      count: 2                  # how many MachineDeploymentTopologies to create
      generateName: workers-    # prefix used to build each topology's name (workers-1, workers-2, ...)
                                 # only used if template.name is not set
      template:                 # a clusterv1.MachineDeploymentTopology
        class: default-worker
        replicas: 1
```

### `edit.machineDeployment.topology.cluster.testctl`

Go type: `plugins.MachineDeploymentEditPlugin`, config: `plugins.MachineDeploymentEditPluginConfig`.

Patches any subset of the selected MachineDeployment's topology fields on the Cluster in a
single patch (so multiple changes, e.g. scaling up and changing taints, are applied
concurrently) and waits for the change to converge. Requires ClusterClass (`spec.topology` must
be defined).

Every config field is optional; a nil field means "don't touch"; a non-nil field means "set the
corresponding MachineDeploymentTopology field to exactly this value":

| Field | Patches | Extra wait behavior |
|---|---|---|
| `metadata` | topology entry's `metadata` | waits for the labels/annotations to appear on the MachineDeployment |
| `class` | topology entry's `class` | — |
| `failureDomain` | topology entry's `failureDomain` | — |
| `replicas` | topology entry's `replicas` | waits for the MachineDeployment to reach the target replica count |
| `healthCheck` | topology entry's `healthCheck` | waits for the corresponding MachineHealthCheck to reflect the desired checks/remediation (or to be absent, if disabled) |
| `deletion` | topology entry's `deletion` | — |
| `taints` | topology entry's `taints` | waits for every Machine's `spec.taints` to match |
| `minReadySeconds` | topology entry's `minReadySeconds` | — |
| `readinessGates` | topology entry's `readinessGates` | — |
| `rollout` | topology entry's `rollout` | — |
| `variables` | topology entry's `variables` | — |

Regardless of which fields are set, the plugin always waits for every Machine of the
MachineDeployment to have the `Available` condition `True`, and for the MachineDeployment
itself to be `Available` and not `RollingOut` (i.e. not updating in place) before returning.

```yaml
- plugin: select.machineDeployment.testctl
  run:
    - plugin: edit.machineDeployment.topology.cluster.testctl
      replicas: 5
```

### `delete.machineDeployment.topology.cluster.testctl`

Go type: `plugins.MachineDeploymentDeletePlugin` (no config).

Removes the selected MachineDeployment's topology entry from
`spec.topology.workers.machineDeployments` and waits for the MachineDeployment object itself to
be deleted.

```yaml
- plugin: select.machineDeployment.testctl
  run:
    - plugin: delete.machineDeployment.topology.cluster.testctl
```

### `install.helmChart.testctl`

Go type: `plugins.HelmChartInstallPlugin`, config: `plugins.HelmChartInstallPluginConfig`.

Installs a Helm chart using `helm upgrade --install`. By default, an existing release is
skipped (checked via `helm status`). Set `upgrade: true` on the plugin to upgrade an existing
release using the configured chart, version, and values. Missing releases are installed in
either mode.
If nested under`select.cluster.testctl`, it targets the selected **workload** cluster,
otherwise it targets the **management** cluster testctl is connected to.

```yaml
- plugin: install.helmChart.testctl   # top level: applies to the management cluster
  releaseName: my-release
  chart: my-chart
  repoURL: https://example.com/charts   # optional, if chart is a repo chart name
  version: 1.2.3                        # optional
  upgrade: false                        # optional, set true to upgrade an existing release
  namespace: my-namespace               # optional, defaults to "default"
  values:                               # optional, arbitrary inline chart values
    key: value
  insecure: false                       # optional, maps to helm's --plain-http (OCI registries only)

- plugin: select.cluster.testctl
  run:
    - plugin: install.helmChart.testctl   # nested: applies to the workload cluster
      releaseName: my-release
      chart: my-chart
```

### `uninstall.helmChart.testctl`

Go type: `plugins.HelmChartUninstallPlugin`, config: `plugins.HelmChartUninstallPluginConfig`.

Uninstalls a Helm release by shelling out to the `helm` CLI. Skips entirely if the release
doesn't exist.
If nested under `select.cluster.testctl`, it targets the selected **workload** cluster,
otherwise it targets the **management** cluster testctl is connected to.

```yaml
- plugin: uninstall.helmChart.testctl   # top level: targets the management cluster
  releaseName: my-release
  namespace: my-namespace                # optional, defaults to "default"

- plugin: select.cluster.testctl
  run:
    - plugin: uninstall.helmChart.testctl   # nested: targets the workload cluster
      releaseName: my-release
```

### `apply.manifest.testctl`

Go type: `plugins.ManifestApplyPlugin`, config: `plugins.ManifestApplyPluginConfig`.

Substitutes a YAML manifest file with the `envsubst` CLI, then applies it by shelling out to
`kubectl apply`. Skips entirely if every object in the manifest already exists on the target
cluster.
If nested under`select.cluster.testctl`, it targets the selected **workload** cluster,
otherwise it targets the **management** cluster testctl is connected to.

```yaml
- plugin: apply.manifest.testctl   # top level: applies to the management cluster
  path: ./manifests/my-manifest.yaml
  env:                              # optional, extra vars for envsubst, in addition to
                                     # (and overriding, on conflict) the process environment
    - name: MY_VAR
      value: my-value

- plugin: select.cluster.testctl
  run:
    - plugin: apply.manifest.testctl   # nested: applies to the workload cluster
      path: ./manifests/my-manifest.yaml
```

### Putting it together

A more complete example, scaling up a Cluster's control plane and workers, then rolling back
on failure, then upgrading:

```yaml
runConfig:
  dryRun: false

run:
  - plugin: select.cluster.testctl
    nameRegex: "^my-cluster-"
    run:
      - plugin: select.controlPlane.testctl
        run:
          - plugin: edit.controlPlane.topology.cluster.testctl
            replicas: 3

      - plugin: create.machineDeployment.topology.cluster.testctl
        count: 1
        generateName: workers-
        template:
          class: default-worker
          replicas: 2
        runOnError:
          - plugin: select.machineDeployment.testctl
            topologyNameRegex: "^workers-"
            run:
              - plugin: delete.machineDeployment.topology.cluster.testctl

      - plugin: edit.topology.cluster.testctl
        version: v1.31.0
    
    runConfig:
      concurrency: 2
```

## Writing a new plugin

Plugins live in the `plugins` package (one file per plugin, e.g. `cluster_delete.go`) and are
built from the interfaces defined in `core/plugins.go`. A plugin implements one or more of:

- `core.SelectorPlugin` — selects a set of objects to act on (returns `[]core.TestObjects`).
  Implement this when your plugin needs to fan out over a list of objects for its nested `run`.
- `core.ExecutorPlugin` — executes an action against a single `core.TestObjects`. Implement
  this for a leaf action (scale, delete, upgrade, ...).
- `core.ConfigurablePlugin` — parses the plugin's own YAML config into a typed struct
  (`ParseConfig`, using `sigs.k8s.io/yaml`'s `UnmarshalStrict`). Optional — omit it if your
  plugin doesn't take any config (e.g. `ClusterDeletePlugin`).
- `core.MessageGeneratorPlugin` — generates the human-readable message shown by `debug` /
  `debugOnError` prompts. Optional but recommended for any non-trivial plugin.
- `core.CallStackValidatorPlugin` — validates that the plugin is only used as a (possibly
  indirect) child of a specific ancestor plugin, e.g. requiring `select.cluster.testctl` before
  `delete.cluster.testctl`. Implement this whenever your plugin depends on a `TestObjects` key
  set by a specific selector.

To add a new plugin:

1. Create `plugins/<noun>_<verb>.go` following the existing file layout.
2. Pick a key of the form `<verb>.<noun>.testctl` (see the [reference table](#plugin-reference)
   for existing keys) and define it as a package constant.
3. Define your plugin type and, if needed, its config type (`<Name>PluginConfig`), documenting
   every exported field with a doc comment.
4. Implement the interfaces you need, add compile-time assertions (`var _ core.ExecutorPlugin =
   &MyPlugin{}`, etc.) right after the type declaration, matching the style used by every other
   plugin.
5. Register the plugin in an `init()` function: `registerPlugin(myPluginKey, &MyPlugin{})`.
6. Reuse the shared helpers in `plugins/util.go` (`getClusters`, `getControlPlane`,
   `getMachineDeployments`, `getMachineDeploymentMachines`, `waitForControlPlaneMachines`,
   `waitForMachineDeploymentMachines`, ...) and the `Unwrap*TestObject`/`Get*Object` accessors
   defined alongside each selector plugin (`UnwrapClusterTestObject`, `UnwrapControlPlaneTestObject`,
   `UnwrapMachineDeploymentTestObject`) instead of re-fetching or re-casting objects from
   `TestObjects`.

Minimal skeleton for an executor plugin with no config, modeled on
`plugins/cluster_delete.go`:

```go
package plugins

import (
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const myActionPluginKey = "myAction.cluster.testctl"

// MyActionPlugin can be used to <describe the action>.
type MyActionPlugin struct{}

var _ core.ExecutorPlugin = &MyActionPlugin{}
var _ core.MessageGeneratorPlugin = &MyActionPlugin{}
var _ core.CallStackValidatorPlugin = &MyActionPlugin{}

func init() {
	registerPlugin(myActionPluginKey, &MyActionPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p MyActionPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, _ any, runConfig core.RunConfig) error {
	cluster, err := UnwrapClusterTestObject(objects)
	if err != nil {
		return err
	}

	// ... do the work, respecting runConfig.DryRun / runConfig.SkipWait ...
	return nil
}

// GenerateMessage generates a message text for the plugin call.
func (p MyActionPlugin) GenerateMessage(objects core.TestObjects, _ any) (string, error) {
	cluster, err := UnwrapClusterTestObject(objects)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("My action on Cluster %s", klog.KObj(cluster)), nil
}

// ValidateCallStack validates the call stack for the plugin.
func (p MyActionPlugin) ValidateCallStack(_ context.Context, callStack []string) error {
	if !slices.Contains(callStack, ClusterSelectorPluginKey) {
		return errors.Errorf("this plugin can only be called as a child of %s", ClusterSelectorPluginKey)
	}
	return nil
}
```

## Known limitations

- `create.machineDeployment.topology.cluster.testctl` cannot be simulated in dry-run mode: since nested `run`
  steps act on the MachineDeployments it would have created, dry-run skips both the creation
  and any nested `run` entirely for that entry.
- `install.helmChart.testctl`/`uninstall.helmChart.testctl` and `apply.manifest.testctl` require `helm`/`kubectl`/`envsubst` on
  `PATH` (or the `TESTCTL_HELM_BIN`/`TESTCTL_KUBECTL_BIN`/`TESTCTL_ENVSUBST_BIN` env var
  overrides).


go run ../cluster-api-provider-vsphere/hack/tools/testctl --config ./tmp/yaml/testctl/foo.yaml