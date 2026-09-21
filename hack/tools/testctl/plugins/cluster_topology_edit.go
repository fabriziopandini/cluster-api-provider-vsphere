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

const topologyEditPluginKey = "edit.topology.cluster.testctl"

// TopologyEditPlugin can be used to edit the Cluster's spec.topology (version, classRef, variables).
type TopologyEditPlugin struct{}

var _ core.ExecutorPlugin = &TopologyEditPlugin{}
var _ core.ConfigurablePlugin = &TopologyEditPlugin{}
var _ core.MessageGeneratorPlugin = &TopologyEditPlugin{}
var _ core.CallStackValidatorPlugin = &TopologyEditPlugin{}

// TopologyEditPluginConfig defines the config for the TopologyEditPlugin.
//
// Every field is optional; a nil field means "don't touch", a non-nil field means "set the
// corresponding cluster.Spec.Topology field to exactly this value". Any subset of fields can be
// set at once and is applied in a single patch, so changes can be performed concurrently (e.g.
// upgrading the version and rebasing to a new ClusterClass at the same time).
//
// spec.topology.controlPlane and spec.topology.workers.machineDeployments fields are out of
// scope for this plugin: they're handled by edit.controlPlane.topology.cluster.testctl and
// edit.machineDeployment.topology.cluster.testctl.
type TopologyEditPluginConfig struct {
	// Version is the Kubernetes version of the cluster.
	Version *string `json:"version,omitempty"`

	// ClassRef is the ref to the ClusterClass that should be used for the topology.
	ClassRef *clusterv1.ClusterClassRef `json:"classRef,omitempty"`

	// Variables overrides cluster.Spec.Topology.Variables.
	Variables *[]clusterv1.ClusterVariable `json:"variables,omitempty"`
}

func init() {
	registerPlugin(topologyEditPluginKey, &TopologyEditPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p TopologyEditPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
	config, cluster, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return err
	}

	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// TODO: check if the Cluster is already fully at the desired state so we can still wait for the operation to complete when necessary.
	changes := pendingTopologyChanges(cluster, config)
	if len(changes) == 0 {
		log.Info(fmt.Sprintf("Editing topology of Cluster %s action skipped, Cluster already matches the desired state", klog.KObj(cluster)))
		return nil
	}

	log.Info(fmt.Sprintf("Editing topology of Cluster %s: %s", klog.KObj(cluster), strings.Join(changes, ", ")))
	if ptr.Deref(runConfig.DryRun, false) {
		return nil
	}

	original := cluster.DeepCopy()
	applyTopologyChanges(cluster, config)
	if err := c.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return pkgerrors.Wrapf(err, "failed to patch Cluster %s", klog.KObj(cluster))
	}

	if ptr.Deref(runConfig.SkipWait, false) {
		return nil
	}

	log.Info(fmt.Sprintf("Waiting for Cluster %s to apply desired changes: %s", klog.KObj(cluster), strings.Join(changes, ", ")))
	var retryErr error
	timeout := 5 * time.Minute
	if runConfig.Timeout != nil {
		timeout = runConfig.Timeout.Duration
	}

	var clusterClass *clusterv1.ClusterClass
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, func(ctx context.Context) (done bool, err error) {
		retryErr = nil

		// Note: the ClusterClass is re-fetched on every iteration because a classRef change
		// means the desired state must be read from the new ClusterClass, not the one selected
		// before this plugin ran.
		clusterClass, err = getClusterClass(ctx, c, cluster)
		if err != nil {
			return false, err
		}

		version := cluster.Spec.Topology.Version

		controlPlane, err := getControlPlane(ctx, c, cluster)
		if err != nil {
			return false, err
		}
		controlPlaneTopology := &cluster.Spec.Topology.ControlPlane
		controlPlaneClass := &clusterClass.Spec.ControlPlane

		if done, retryErr = waitForControlPlane(controlPlane, version, controlPlaneTopology, controlPlaneClass); !done {
			return false, nil
		}

		if done, err, retryErr = waitForControlPlaneMachines(ctx, c, cluster, controlPlane, ptr.Deref(controlPlaneTopology.Replicas, 0), version); err != nil || retryErr != nil {
			return done, err
		}

		machineDeployments, err := getMachineDeployments(ctx, c, cluster, "", nil)
		if err != nil {
			return false, err
		}

		for _, machineDeployment := range machineDeployments {
			mdTopologyName, ok := machineDeployment.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
			if !ok {
				return false, pkgerrors.Errorf("MachineDeployment %s doesn't have the %s label", klog.KObj(machineDeployment), clusterv1.ClusterTopologyMachineDeploymentNameLabel)
			}

			mdTopologyIndex := getMachineDeploymentTopologyIndex(cluster, mdTopologyName)
			if mdTopologyIndex == -1 {
				return false, pkgerrors.Errorf("cannot find a MachineDeployment with name %s in cluster.spec.topology.workers.machineDeployments", mdTopologyName)
			}
			mdTopology := &cluster.Spec.Topology.Workers.MachineDeployments[mdTopologyIndex]

			mdClass, err := findMachineDeploymentClass(clusterClass, mdTopology.Class)
			if err != nil {
				return false, err
			}

			if done, retryErr = waitForMachineDeployment(machineDeployment, version, mdTopology, mdClass); !done {
				return false, nil
			}

			if done, err, retryErr = waitForMachineDeploymentMachines(ctx, c, cluster, machineDeployment, ptr.Deref(mdTopology.Replicas, 0), version); err != nil || retryErr != nil {
				return done, err
			}
		}

		// Note: it is not possible to check if changes to topology.Variables have been
		// propagated, because their impact is defined by patches.
		return true, nil
	}); err != nil || retryErr != nil {
		return kerrors.NewAggregate([]error{retryErr, err})
	}

	// Since the cluster class might be changed, override it.
	objects[ClusterClassObjectKey] = clusterClass

	log.Info(fmt.Sprintf("Editing topology of Cluster %s completed: %s", klog.KObj(cluster), strings.Join(changes, ", ")))

	return nil
}

// ParseConfig parses the config for this plugin.
func (p TopologyEditPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &TopologyEditPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}
	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p TopologyEditPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config, cluster, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return "", err
	}

	changes := pendingTopologyChanges(cluster, config)
	if len(changes) == 0 {
		return fmt.Sprintf("Edit topology of Cluster %s: no changes", klog.KObj(cluster)), nil
	}
	return fmt.Sprintf("Edit topology of Cluster %s: %s", klog.KObj(cluster), strings.Join(changes, ", ")), nil
}

// ValidateCallStack validates the call stack for the plugin.
func (p TopologyEditPlugin) ValidateCallStack(_ context.Context, callStack []string) error {
	if !slices.Contains(callStack, clusterSelectorPluginKey) {
		return pkgerrors.Errorf("this plugin can only be called as a child of %s", clusterSelectorPluginKey)
	}
	return nil
}

func (p TopologyEditPlugin) unwrapConfigAndTestObjects(objects core.TestObjects, pluginConfigUntyped any) (*TopologyEditPluginConfig, *clusterv1.Cluster, error) {
	config := &TopologyEditPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*TopologyEditPluginConfig)
	}

	cluster, err := UnwrapClusterTestObject(objects)
	if err != nil {
		return config, nil, err
	}

	if !cluster.Spec.Topology.IsDefined() {
		return config, nil, pkgerrors.Errorf("unable to edit topology for Cluster %s. support for clusters without spec.topology not implemented yet", klog.KObj(cluster))
	}
	return config, cluster, nil
}

func pendingTopologyChanges(cluster *clusterv1.Cluster, config *TopologyEditPluginConfig) []string {
	var changes []string

	if config.Version != nil && cluster.Spec.Topology.Version != *config.Version {
		changes = append(changes, fmt.Sprintf("version %s->%s", cluster.Spec.Topology.Version, *config.Version))
	}
	if config.ClassRef != nil && !reflect.DeepEqual(cluster.Spec.Topology.ClassRef, *config.ClassRef) {
		changes = append(changes, "classRef updated")
	}
	if config.Variables != nil && !reflect.DeepEqual(cluster.Spec.Topology.Variables, *config.Variables) {
		changes = append(changes, "variables updated")
	}

	return changes
}

func applyTopologyChanges(cluster *clusterv1.Cluster, config *TopologyEditPluginConfig) {
	if config.Version != nil {
		cluster.Spec.Topology.Version = *config.Version
	}
	if config.ClassRef != nil {
		cluster.Spec.Topology.ClassRef = *config.ClassRef
	}
	if config.Variables != nil {
		cluster.Spec.Topology.Variables = *config.Variables
	}
}
