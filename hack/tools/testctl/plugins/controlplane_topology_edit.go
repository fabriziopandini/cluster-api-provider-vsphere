/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package plugins

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const controlPlaneEditPluginKey = "edit.controlPlane.topology.cluster.testctl"

// ControlPlaneEditPlugin can be used to edit the Cluster's spec.topology.controlPlane.
type ControlPlaneEditPlugin struct{}

var _ core.ExecutorPlugin = &ControlPlaneEditPlugin{}
var _ core.ConfigurablePlugin = &ControlPlaneEditPlugin{}
var _ core.MessageGeneratorPlugin = &ControlPlaneEditPlugin{}
var _ core.CallStackValidatorPlugin = &ControlPlaneEditPlugin{}

// ControlPlaneEditPluginConfig defines the config for the ControlPlaneEditPlugin.
//
// Every field is optional; a nil field means "don't touch", a non-nil field means "set the
// corresponding cluster.Spec.Topology.ControlPlane field to exactly this value". Any subset of
// fields can be set at once and is applied in a single patch, so changes can be performed
// concurrently (e.g. scale up and change taints at the same time).
//
// Kubernetes version upgrades are out of scope for this plugin: cluster.Spec.Topology.Version
// is a Cluster-wide field shared with the workers, and is handled by
// edit.topology.cluster.testctl.
type ControlPlaneEditPluginConfig struct {
	// Metadata overrides cluster.Spec.Topology.ControlPlane.Metadata.
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Replicas is the target number of KubeadmControlPlane replicas.
	Replicas *int32 `json:"replicas,omitempty"`

	// Rollout overrides cluster.Spec.Topology.ControlPlane.Rollout.
	Rollout *clusterv1.ControlPlaneTopologyRolloutSpec `json:"rollout,omitempty"`

	// HealthCheck overrides cluster.Spec.Topology.ControlPlane.HealthCheck.
	HealthCheck *clusterv1.ControlPlaneTopologyHealthCheck `json:"healthCheck,omitempty"`

	// Deletion overrides cluster.Spec.Topology.ControlPlane.Deletion.
	Deletion *clusterv1.ControlPlaneTopologyMachineDeletionSpec `json:"deletion,omitempty"`

	// Taints overrides cluster.Spec.Topology.ControlPlane.Taints.
	Taints *[]clusterv1.MachineTaint `json:"taints,omitempty"`

	// ReadinessGates overrides cluster.Spec.Topology.ControlPlane.ReadinessGates.
	ReadinessGates *[]clusterv1.MachineReadinessGate `json:"readinessGates,omitempty"`

	// Variables overrides cluster.Spec.Topology.ControlPlane.Variables.
	Variables *clusterv1.ControlPlaneVariables `json:"variables,omitempty"`
}

func init() {
	registerPlugin(controlPlaneEditPluginKey, &ControlPlaneEditPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p ControlPlaneEditPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
	config, cluster, topology, class, controlPlane, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return err
	}

	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(cluster), "KubeadmControlPlane", klog.KObj(controlPlane))
	ctx = ctrl.LoggerInto(ctx, log)

	// TODO: check if the control plane machines are already at the desired state so we can still wait for the operation to complete when necessary.
	changes := pendingControlPlaneChanges(topology, config)
	if len(changes) == 0 {
		log.Info(fmt.Sprintf("Editing KubeadmControlPlane %s in Cluster %s action skipped, ControlPlane already matches the desired state", klog.KObj(controlPlane), klog.KObj(cluster)))
		return nil
	}

	log.Info(fmt.Sprintf("Editing KubeadmControlPlane %s in Cluster %s: %s", klog.KObj(controlPlane), klog.KObj(cluster), strings.Join(changes, ", ")))
	if ptr.Deref(runConfig.DryRun, false) {
		return nil
	}

	original := cluster.DeepCopy()
	applyControlPlaneChanges(topology, config)
	if err := c.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return pkgerrors.Wrapf(err, "failed to patch Cluster %s", klog.KObj(cluster))
	}

	if ptr.Deref(runConfig.SkipWait, false) {
		return nil
	}

	log.Info(fmt.Sprintf("Waiting for KubeadmControlPlane %s in Cluster %s to apply desired changes: %s", klog.KObj(controlPlane), klog.KObj(cluster), strings.Join(changes, ", ")))
	replicas := ptr.Deref(topology.Replicas, 0)
	version := cluster.Spec.Topology.Version
	var retryErr error
	timeout := 5 * time.Minute
	if runConfig.Timeout != nil {
		timeout = runConfig.Timeout.Duration
	}
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, func(ctx context.Context) (done bool, err error) {
		retryErr = nil

		controlPlane, err = getControlPlane(ctx, c, cluster)
		if err != nil {
			return false, err
		}

		if done, retryErr = waitForControlPlane(controlPlane, version, topology, class); !done {
			return false, nil
		}

		if done, err, retryErr = waitForControlPlaneMachineHealthCheck(ctx, c, controlPlane, topology.HealthCheck, class.HealthCheck); err != nil || !done {
			return done, err
		}

		if done, err, retryErr = waitForControlPlaneMachines(ctx, c, cluster, controlPlane, replicas, version,
			newMachineMetadataCheck(topology.Metadata, class.Metadata),
			newReadinessGatesCheck(topology.ReadinessGates, class.ReadinessGates),
			newTaintsCheck(topology.Taints, class.Taints),
			newControlPlaneDeletionCheck(topology.Deletion, class.Deletion),
			// Note: no failureDomain check (assigned Machine by Machine) and minReadySeconds check (not implemented in KubeadmControlPlane).
		); err != nil || retryErr != nil {
			return done, err
		}

		return true, nil
	}); err != nil || retryErr != nil {
		return kerrors.NewAggregate([]error{retryErr, err})
	}

	log.Info(fmt.Sprintf("Editing KubeadmControlPlane %s in Cluster %s completed: %s", klog.KObj(controlPlane), klog.KObj(cluster), strings.Join(changes, ", ")))

	return nil
}

// ParseConfig parses the config for this plugin.
func (p ControlPlaneEditPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &ControlPlaneEditPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}
	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p ControlPlaneEditPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config, cluster, topology, _, controlPlane, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return "", err
	}

	changes := pendingControlPlaneChanges(topology, config)
	if len(changes) == 0 {
		return fmt.Sprintf("Edit KubeadmControlPlane %s in Cluster %s: no changes", klog.KObj(controlPlane), klog.KObj(cluster)), nil
	}
	return fmt.Sprintf("Edit KubeadmControlPlane %s in Cluster %s: %s", klog.KObj(controlPlane), klog.KObj(cluster), strings.Join(changes, ", ")), nil
}

// ValidateCallStack validates the call stack for the plugin.
func (p ControlPlaneEditPlugin) ValidateCallStack(_ context.Context, callStack []string) error {
	if !slices.Contains(callStack, controlPlaneSelectorPluginKey) {
		return pkgerrors.Errorf("this plugin can only be called as a child of %s", controlPlaneSelectorPluginKey)
	}
	return nil
}

func (p ControlPlaneEditPlugin) unwrapConfigAndTestObjects(objects core.TestObjects, pluginConfigUntyped any) (*ControlPlaneEditPluginConfig, *clusterv1.Cluster, *clusterv1.ControlPlaneTopology, *clusterv1.ControlPlaneClass, *controlplanev1.KubeadmControlPlane, error) {
	config := &ControlPlaneEditPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*ControlPlaneEditPluginConfig)
	}

	cluster, err := UnwrapClusterTestObject(objects)
	if err != nil {
		return config, nil, nil, nil, nil, err
	}

	if !cluster.Spec.Topology.IsDefined() {
		return config, nil, nil, nil, nil, pkgerrors.Errorf("unable to edit control plane for Cluster %s. support for clusters without spec.topology not implemented yet", klog.KObj(cluster))
	}

	clusterClass, err := UnwrapClusterClassTestObject(objects)
	if err != nil {
		return config, nil, nil, nil, nil, err
	}

	controlPlane, err := UnwrapControlPlaneTestObject(objects)
	if err != nil {
		return config, nil, nil, nil, nil, err
	}
	return config, cluster, &cluster.Spec.Topology.ControlPlane, &clusterClass.Spec.ControlPlane, controlPlane, nil
}

func pendingControlPlaneChanges(topology *clusterv1.ControlPlaneTopology, config *ControlPlaneEditPluginConfig) []string {
	var changes []string

	if config.Metadata != nil && !reflect.DeepEqual(topology.Metadata, *config.Metadata) {
		changes = append(changes, "metadata updated")
	}
	if config.Replicas != nil && ptr.Deref(topology.Replicas, 0) != *config.Replicas {
		changes = append(changes, fmt.Sprintf("replicas %d->%d", ptr.Deref(topology.Replicas, 0), *config.Replicas))
	}
	if config.Rollout != nil && !reflect.DeepEqual(topology.Rollout, *config.Rollout) {
		changes = append(changes, "rollout updated")
	}
	if config.HealthCheck != nil && !reflect.DeepEqual(topology.HealthCheck, *config.HealthCheck) {
		changes = append(changes, "healthCheck updated")
	}
	if config.Deletion != nil && !reflect.DeepEqual(topology.Deletion, *config.Deletion) {
		changes = append(changes, "deletion updated")
	}
	if config.Taints != nil && !reflect.DeepEqual(topology.Taints, *config.Taints) {
		changes = append(changes, "taints updated")
	}
	if config.ReadinessGates != nil && !reflect.DeepEqual(topology.ReadinessGates, *config.ReadinessGates) {
		changes = append(changes, "readinessGates updated")
	}
	if config.Variables != nil && !reflect.DeepEqual(topology.Variables, *config.Variables) {
		changes = append(changes, "variables updated")
	}

	return changes
}

func applyControlPlaneChanges(topology *clusterv1.ControlPlaneTopology, config *ControlPlaneEditPluginConfig) {
	if config.Metadata != nil {
		topology.Metadata = *config.Metadata
	}
	if config.Replicas != nil {
		topology.Replicas = config.Replicas
	}
	if config.Rollout != nil {
		topology.Rollout = *config.Rollout
	}
	if config.HealthCheck != nil {
		topology.HealthCheck = *config.HealthCheck
	}
	if config.Deletion != nil {
		topology.Deletion = *config.Deletion
	}
	if config.Taints != nil {
		topology.Taints = *config.Taints
	}
	if config.ReadinessGates != nil {
		topology.ReadinessGates = *config.ReadinessGates
	}
	if config.Variables != nil {
		topology.Variables = *config.Variables
	}
}
