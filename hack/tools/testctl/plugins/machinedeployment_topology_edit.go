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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const machineDeploymentEditPluginKey = "edit.machineDeployment.topology.cluster.testctl"

// MachineDeploymentEditPlugin can be used to edit a MachineDeployment topology entry for a Cluster.
type MachineDeploymentEditPlugin struct{}

var _ core.ExecutorPlugin = &MachineDeploymentEditPlugin{}
var _ core.ConfigurablePlugin = &MachineDeploymentEditPlugin{}
var _ core.MessageGeneratorPlugin = &MachineDeploymentEditPlugin{}
var _ core.CallStackValidatorPlugin = &MachineDeploymentEditPlugin{}

// MachineDeploymentEditPluginConfig defines the config for the MachineDeploymentEditPlugin.
//
// Every field is optional; a nil field means "don't touch", a non-nil field means "set the
// corresponding cluster.Spec.Topology.Workers.MachineDeployments[i] field to exactly this
// value". Any subset of fields can be set at once and is applied in a single patch, so changes
// can be performed concurrently (e.g. scale up and change taints at the same time).
//
// The topology's Name is deliberately not editable here: it's how the target MachineDeployment
// entry is resolved, and changing it would break that resolution.
type MachineDeploymentEditPluginConfig struct {
	// Metadata overrides the MachineDeployment topology's Metadata.
	Metadata *clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// Class overrides the MachineDeployment topology's Class.
	Class *string `json:"class,omitempty"`

	// FailureDomain overrides the MachineDeployment topology's FailureDomain.
	FailureDomain *string `json:"failureDomain,omitempty"`

	// Replicas is the target number of replicas for this MachineDeployment.
	Replicas *int32 `json:"replicas,omitempty"`

	// HealthCheck overrides the MachineDeployment topology's HealthCheck.
	HealthCheck *clusterv1.MachineDeploymentTopologyHealthCheck `json:"healthCheck,omitempty"`

	// Deletion overrides the MachineDeployment topology's Deletion.
	Deletion *clusterv1.MachineDeploymentTopologyMachineDeletionSpec `json:"deletion,omitempty"`

	// Taints overrides the MachineDeployment topology's Taints.
	Taints *[]clusterv1.MachineTaint `json:"taints,omitempty"`

	// MinReadySeconds overrides the MachineDeployment topology's MinReadySeconds.
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// ReadinessGates overrides the MachineDeployment topology's ReadinessGates.
	ReadinessGates *[]clusterv1.MachineReadinessGate `json:"readinessGates,omitempty"`

	// Rollout overrides the MachineDeployment topology's Rollout.
	Rollout *clusterv1.MachineDeploymentTopologyRolloutSpec `json:"rollout,omitempty"`

	// Variables overrides the MachineDeployment topology's Variables.
	Variables *clusterv1.MachineDeploymentVariables `json:"variables,omitempty"`
}

func init() {
	registerPlugin(machineDeploymentEditPluginKey, &MachineDeploymentEditPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p MachineDeploymentEditPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
	config, cluster, topology, class, machineDeployment, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return err
	}

	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(cluster), "MachineDeployment", klog.KObj(machineDeployment))
	ctx = ctrl.LoggerInto(ctx, log)

	// TODO: check if the control plane machines are already at the desired state so we can still wait for the operation to complete when necessary.
	changes := pendingMachineDeploymentChanges(topology, config)
	if len(changes) == 0 {
		log.Info(fmt.Sprintf("Editing MachineDeployment %s in Cluster %s action skipped, MachineDeployment already matches the desired state", klog.KObj(machineDeployment), klog.KObj(cluster)))
		return nil
	}

	log.Info(fmt.Sprintf("Editing MachineDeployment %s in Cluster %s: %s", klog.KObj(machineDeployment), klog.KObj(cluster), strings.Join(changes, ", ")))
	if ptr.Deref(runConfig.DryRun, false) {
		return nil
	}

	original := cluster.DeepCopy()
	applyMachineDeploymentChanges(topology, config)
	if err := c.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return pkgerrors.Wrapf(err, "failed to patch Cluster %s", klog.KObj(cluster))
	}

	if ptr.Deref(runConfig.SkipWait, false) {
		return nil
	}

	log.Info(fmt.Sprintf("Waiting for MachineDeployment %s in Cluster %s to apply desired changes: %s", klog.KObj(machineDeployment), klog.KObj(cluster), strings.Join(changes, ", ")))
	replicas := ptr.Deref(topology.Replicas, 0)
	version := cluster.Spec.Topology.Version
	var retryErr error
	timeout := 5 * time.Minute
	if runConfig.Timeout != nil {
		timeout = runConfig.Timeout.Duration
	}
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, func(ctx context.Context) (done bool, err error) {
		retryErr = nil

		machineDeployment, err = getMachineDeployment(ctx, c, cluster, topology.Name)
		if err != nil {
			return false, err
		}
		if machineDeployment == nil {
			retryErr = pkgerrors.Errorf("waiting for MachineDeployment with topology name %s to exist", topology.Name)
			return false, nil
		}

		if done, retryErr = waitForMachineDeployment(machineDeployment, version, topology, class); !done {
			return false, nil
		}

		if done, err, retryErr = waitForMachineDeploymentMachineHealthCheck(ctx, c, machineDeployment, topology.HealthCheck, class.HealthCheck); err != nil || !done {
			return done, err
		}

		if done, err, retryErr = waitForMachineDeploymentMachines(ctx, c, cluster, machineDeployment, replicas, version,
			newMachineMetadataCheck(topology.Metadata, class.Metadata),
			newReadinessGatesCheck(topology.ReadinessGates, class.ReadinessGates),
			newTaintsCheck(topology.Taints, class.Taints),
			newMinReadySecondsCheck(topology.MinReadySeconds, class.MinReadySeconds),
			newFailureDomainCheck(topology.FailureDomain, class.FailureDomain),
			newMachineDeploymentDeletionCheck(topology.Deletion, class.Deletion),
		); err != nil || retryErr != nil {
			return done, err
		}

		return true, nil
	}); err != nil || retryErr != nil {
		return kerrors.NewAggregate([]error{retryErr, err})
	}

	log.Info(fmt.Sprintf("Editing MachineDeployment %s in Cluster %s completed: %s", klog.KObj(machineDeployment), klog.KObj(cluster), strings.Join(changes, ", ")))

	return nil
}

// ParseConfig parses the config for this plugin.
func (p MachineDeploymentEditPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &MachineDeploymentEditPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}
	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p MachineDeploymentEditPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config, cluster, topology, _, machineDeployment, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return "", err
	}

	changes := pendingMachineDeploymentChanges(topology, config)
	if len(changes) == 0 {
		return fmt.Sprintf("Edit MachineDeployment %s in Cluster %s: no changes", klog.KObj(machineDeployment), klog.KObj(cluster)), nil
	}
	return fmt.Sprintf("Edit MachineDeployment %s in Cluster %s: %s", klog.KObj(machineDeployment), klog.KObj(cluster), strings.Join(changes, ", ")), nil
}

// ValidateCallStack validates the call stack for the plugin.
func (p MachineDeploymentEditPlugin) ValidateCallStack(_ context.Context, callStack []string) error {
	if !slices.Contains(callStack, machineDeploymentSelectorPluginKey) {
		return pkgerrors.Errorf("this plugin can only be called as a child of %s", machineDeploymentSelectorPluginKey)
	}
	return nil
}

func (p MachineDeploymentEditPlugin) unwrapConfigAndTestObjects(objects core.TestObjects, pluginConfigUntyped any) (*MachineDeploymentEditPluginConfig, *clusterv1.Cluster, *clusterv1.MachineDeploymentTopology, *clusterv1.MachineDeploymentClass, *clusterv1.MachineDeployment, error) {
	config := &MachineDeploymentEditPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*MachineDeploymentEditPluginConfig)
	}

	cluster, err := UnwrapClusterTestObject(objects)
	if err != nil {
		return config, nil, nil, nil, nil, err
	}

	if !cluster.Spec.Topology.IsDefined() {
		return config, nil, nil, nil, nil, pkgerrors.Errorf("unable to edit MachineDeployment for Cluster %s. support for clusters without spec.topology not implemented yet", klog.KObj(cluster))
	}

	clusterClass, err := UnwrapClusterClassTestObject(objects)
	if err != nil {
		return config, nil, nil, nil, nil, err
	}

	machineDeployment, err := UnwrapMachineDeploymentTestObject(objects)
	if err != nil {
		return config, nil, nil, nil, nil, err
	}

	mdTopologyName, ok := machineDeployment.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
	if !ok {
		return config, nil, nil, nil, nil, pkgerrors.Errorf("MachineDeployment doesn't have the %s label", clusterv1.ClusterTopologyMachineDeploymentNameLabel)
	}

	mdTopologyIndex := getMachineDeploymentTopologyIndex(cluster, mdTopologyName)
	if mdTopologyIndex == -1 {
		return config, nil, nil, nil, nil, pkgerrors.Errorf("Cannot find a MachineDeployment with name %s in cluster.spec.topology.workers.machineDeployments", mdTopologyName)
	}

	machineDeploymentTopology := &cluster.Spec.Topology.Workers.MachineDeployments[mdTopologyIndex]

	var machineDeploymentClass *clusterv1.MachineDeploymentClass
	for _, c := range clusterClass.Spec.Workers.MachineDeployments {
		if c.Class == machineDeploymentTopology.Class {
			machineDeploymentClass = &c
		}
	}
	if machineDeploymentClass == nil {
		return config, nil, nil, nil, nil, pkgerrors.Errorf("Cannot find a MachineDeployment class with name %s in clusterClass.spec.workers.machineDeployments", machineDeploymentTopology.Class)
	}

	return config, cluster, machineDeploymentTopology, machineDeploymentClass, machineDeployment, nil
}

func pendingMachineDeploymentChanges(topology *clusterv1.MachineDeploymentTopology, config *MachineDeploymentEditPluginConfig) []string {
	var changes []string

	if config.Metadata != nil && !reflect.DeepEqual(topology.Metadata, *config.Metadata) {
		changes = append(changes, "metadata updated")
	}
	if config.Class != nil && topology.Class != *config.Class {
		changes = append(changes, fmt.Sprintf("class %s->%s", topology.Class, *config.Class))
	}
	if config.FailureDomain != nil && topology.FailureDomain != *config.FailureDomain {
		changes = append(changes, fmt.Sprintf("failureDomain %s->%s", topology.FailureDomain, *config.FailureDomain))
	}
	if config.Replicas != nil && ptr.Deref(topology.Replicas, 0) != *config.Replicas {
		changes = append(changes, fmt.Sprintf("replicas %d->%d", ptr.Deref(topology.Replicas, 0), *config.Replicas))
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
	if config.MinReadySeconds != nil && ptr.Deref(topology.MinReadySeconds, 0) != *config.MinReadySeconds {
		changes = append(changes, fmt.Sprintf("minReadySeconds %d->%d", ptr.Deref(topology.MinReadySeconds, 0), *config.MinReadySeconds))
	}
	if config.ReadinessGates != nil && !reflect.DeepEqual(topology.ReadinessGates, *config.ReadinessGates) {
		changes = append(changes, "readinessGates updated")
	}
	if config.Rollout != nil && !reflect.DeepEqual(topology.Rollout, *config.Rollout) {
		changes = append(changes, "rollout updated")
	}
	if config.Variables != nil && !reflect.DeepEqual(topology.Variables, *config.Variables) {
		changes = append(changes, "variables updated")
	}

	return changes
}

func applyMachineDeploymentChanges(topology *clusterv1.MachineDeploymentTopology, config *MachineDeploymentEditPluginConfig) {
	if config.Metadata != nil {
		topology.Metadata = *config.Metadata
	}
	if config.Class != nil {
		topology.Class = *config.Class
	}
	if config.FailureDomain != nil {
		topology.FailureDomain = *config.FailureDomain
	}
	if config.Replicas != nil {
		topology.Replicas = config.Replicas
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
	if config.MinReadySeconds != nil {
		topology.MinReadySeconds = config.MinReadySeconds
	}
	if config.ReadinessGates != nil {
		topology.ReadinessGates = *config.ReadinessGates
	}
	if config.Rollout != nil {
		topology.Rollout = *config.Rollout
	}
	if config.Variables != nil {
		topology.Variables = *config.Variables
	}
}
