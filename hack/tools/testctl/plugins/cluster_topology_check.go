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
	"slices"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const topologyCheckPluginKey = "check.topology.cluster.testctl"

// TopologyCheckType identifies a single check performed by TopologyCheckPlugin.
type TopologyCheckType string

const (
	// TopologyCheckAvailability checks that the Cluster's own Available condition is True.
	TopologyCheckAvailability TopologyCheckType = "Availability"

	// TopologyCheckControlPlane checks that the KubeadmControlPlane (and its Machines) match
	// the desired topology/class state, the same checks performed by
	// edit.controlPlane.topology.cluster.testctl.
	TopologyCheckControlPlane TopologyCheckType = "ControlPlane"

	// TopologyCheckMachineDeployments checks that every MachineDeployment (and its Machines)
	// matches the desired topology/class state, the same checks performed by
	// edit.machineDeployment.topology.cluster.testctl.
	TopologyCheckMachineDeployments TopologyCheckType = "MachineDeployments"
)

var topologyCheckTypes = sets.Set[TopologyCheckType]{}.Insert(TopologyCheckAvailability, TopologyCheckControlPlane, TopologyCheckMachineDeployments)

// TopologyCheckPlugin can be used to check that a Cluster's topology is in the desired state,
// without performing any mutation.
type TopologyCheckPlugin struct{}

var _ core.ExecutorPlugin = &TopologyCheckPlugin{}
var _ core.ConfigurablePlugin = &TopologyCheckPlugin{}
var _ core.MessageGeneratorPlugin = &TopologyCheckPlugin{}
var _ core.CallStackValidatorPlugin = &TopologyCheckPlugin{}

// TopologyCheckPluginConfig defines the config for the TopologyCheckPlugin.
type TopologyCheckPluginConfig struct {
	// Checks is the list of checks to perform. If empty, every check is performed.
	// One or more of "Availability", "ControlPlane", "MachineDeployments".
	Checks []TopologyCheckType `json:"checks,omitempty"`
}

func init() {
	registerPlugin(topologyCheckPluginKey, &TopologyCheckPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p TopologyCheckPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
	config, cluster, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return err
	}

	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	log.Info(fmt.Sprintf("Checking Cluster %s topology: %s", klog.KObj(cluster), joinTopologyChecks(config.Checks)))
	if ptr.Deref(runConfig.DryRun, false) {
		return nil
	}

	if ptr.Deref(runConfig.SkipWait, false) {
		return nil
	}

	var retryErr error
	checkFunc := func(ctx context.Context) (done bool, err error) {
		retryErr = nil
		defer func() {
			if retryErr != nil {
				log.Info(retryErr.Error())
			}
		}()

		if err := c.Get(ctx, client.ObjectKeyFromObject(cluster), cluster); err != nil {
			return false, pkgerrors.Wrapf(err, "failed to get Cluster %s", klog.KObj(cluster))
		}

		if slices.Contains(config.Checks, TopologyCheckControlPlane) {
			if done, err, retryErr = checkControlPlaneTopology(ctx, c, cluster); err != nil || !done {
				return done, err
			}
		}

		if slices.Contains(config.Checks, TopologyCheckMachineDeployments) {
			if done, err, retryErr = checkMachineDeploymentsTopology(ctx, c, cluster); err != nil || !done {
				return done, err
			}
		}

		if slices.Contains(config.Checks, TopologyCheckAvailability) {
			if done, retryErr = checkClusterAvailability(ctx, c, cluster); !done {
				return false, nil
			}
		}

		return true, nil
	}

	timeout := 5 * time.Minute
	if runConfig.Timeout != nil {
		timeout = runConfig.Timeout.Duration
	}
	if err := wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, false, checkFunc); err != nil || retryErr != nil {
		return kerrors.NewAggregate([]error{retryErr, err})
	}

	log.Info(fmt.Sprintf("Checking Cluster %s topology completed: %s", klog.KObj(cluster), joinTopologyChecks(config.Checks)))

	return nil
}

// checkClusterAvailability returns done=true if the Cluster's own Available condition is True.
func checkClusterAvailability(_ context.Context, _ client.Client, cluster *clusterv1.Cluster) (done bool, retryErr error) {
	if !conditions.IsTrue(cluster, clusterv1.AvailableCondition) {
		return false, pkgerrors.Errorf("waiting for Cluster %s to have Available condition True", klog.KObj(cluster))
	}
	return true, nil
}

// checkControlPlaneTopology performs the same checks as
// edit.controlPlane.topology.cluster.testctl (waitForControlPlane,
// waitForControlPlaneMachineHealthCheck, waitForControlPlaneMachines).
func checkControlPlaneTopology(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (done bool, err error, retryErr error) {
	if !cluster.Spec.ControlPlaneRef.IsDefined() {
		return false, nil, pkgerrors.Errorf("waiting for cluster.spec.controlPlaneRef to be set")
	}

	clusterClass, err := getClusterClass(ctx, c, cluster)
	if err != nil {
		return false, err, nil
	}

	controlPlane, err := getControlPlane(ctx, c, cluster)
	if err != nil {
		return false, err, nil
	}

	topology := &cluster.Spec.Topology.ControlPlane
	class := &clusterClass.Spec.ControlPlane
	version := cluster.Spec.Topology.Version

	if done, retryErr = waitForControlPlane(controlPlane, version, topology, class); !done {
		return false, nil, retryErr
	}

	if done, err, retryErr = waitForControlPlaneMachineHealthCheck(ctx, c, controlPlane, topology.HealthCheck, class.HealthCheck); err != nil || !done {
		return done, err, retryErr
	}

	if done, err, retryErr = waitForControlPlaneMachines(ctx, c, cluster, controlPlane, ptr.Deref(topology.Replicas, 0), version,
		newMachineMetadataCheck(topology.Metadata, class.Metadata),
		newReadinessGatesCheck(topology.ReadinessGates, class.ReadinessGates),
		newTaintsCheck(topology.Taints, class.Taints),
		newControlPlaneDeletionCheck(topology.Deletion, class.Deletion),
	); err != nil || !done {
		return done, err, retryErr
	}

	return true, nil, nil
}

// checkMachineDeploymentsTopology performs the same checks as
// edit.machineDeployment.topology.cluster.testctl (waitForMachineDeployment,
// waitForMachineDeploymentMachineHealthCheck, waitForMachineDeploymentMachines) for every
// MachineDeployment of the Cluster.
func checkMachineDeploymentsTopology(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (done bool, err error, retryErr error) {
	clusterClass, err := getClusterClass(ctx, c, cluster)
	if err != nil {
		return false, err, nil
	}

	machineDeployments, err := getMachineDeployments(ctx, c, cluster, "", nil)
	if err != nil {
		return false, err, nil
	}

	version := cluster.Spec.Topology.Version

	for _, machineDeployment := range machineDeployments {
		mdTopologyName, ok := machineDeployment.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
		if !ok {
			return false, pkgerrors.Errorf("MachineDeployment %s doesn't have the %s label", klog.KObj(machineDeployment), clusterv1.ClusterTopologyMachineDeploymentNameLabel), nil
		}

		mdTopologyIndex := getMachineDeploymentTopologyIndex(cluster, mdTopologyName)
		if mdTopologyIndex == -1 {
			return false, pkgerrors.Errorf("cannot find a MachineDeployment with name %s in cluster.spec.topology.workers.machineDeployments", mdTopologyName), nil
		}
		mdTopology := &cluster.Spec.Topology.Workers.MachineDeployments[mdTopologyIndex]

		mdClass, err := findMachineDeploymentClass(clusterClass, mdTopology.Class)
		if err != nil {
			return false, err, nil
		}

		if done, retryErr = waitForMachineDeployment(machineDeployment, version, mdTopology, mdClass); !done {
			return false, nil, retryErr
		}

		if done, err, retryErr = waitForMachineDeploymentMachineHealthCheck(ctx, c, machineDeployment, mdTopology.HealthCheck, mdClass.HealthCheck); err != nil || !done {
			return done, err, retryErr
		}

		if done, err, retryErr = waitForMachineDeploymentMachines(ctx, c, cluster, machineDeployment, ptr.Deref(mdTopology.Replicas, 0), version,
			newMachineMetadataCheck(mdTopology.Metadata, mdClass.Metadata),
			newReadinessGatesCheck(mdTopology.ReadinessGates, mdClass.ReadinessGates),
			newTaintsCheck(mdTopology.Taints, mdClass.Taints),
			newMinReadySecondsCheck(mdTopology.MinReadySeconds, mdClass.MinReadySeconds),
			newFailureDomainCheck(mdTopology.FailureDomain, mdClass.FailureDomain),
			newMachineDeploymentDeletionCheck(mdTopology.Deletion, mdClass.Deletion),
		); err != nil || !done {
			return done, err, retryErr
		}
	}

	return true, nil, nil
}

// ParseConfig parses the config for this plugin.
func (p TopologyCheckPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &TopologyCheckPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}

	if len(config.Checks) == 0 {
		config.Checks = []TopologyCheckType{TopologyCheckAvailability, TopologyCheckControlPlane, TopologyCheckMachineDeployments}
	}

	for _, check := range config.Checks {
		if !topologyCheckTypes.Has(check) {
			return nil, pkgerrors.Errorf("invalid check %q, must be one of %s", check, sets.List(topologyCheckTypes))
		}
	}

	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p TopologyCheckPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config, cluster, err := p.unwrapConfigAndTestObjects(objects, pluginConfigUntyped)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Check Cluster %s topology: %s", klog.KObj(cluster), joinTopologyChecks(config.Checks)), nil
}

// ValidateCallStack validates the call stack for the plugin.
func (p TopologyCheckPlugin) ValidateCallStack(_ context.Context, callStack []string) error {
	if !slices.Contains(callStack, clusterSelectorPluginKey) {
		return pkgerrors.Errorf("this plugin can only be called as a child of %s", clusterSelectorPluginKey)
	}
	return nil
}

func (p TopologyCheckPlugin) unwrapConfigAndTestObjects(objects core.TestObjects, pluginConfigUntyped any) (*TopologyCheckPluginConfig, *clusterv1.Cluster, error) {
	config := &TopologyCheckPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*TopologyCheckPluginConfig)
	}

	cluster, err := UnwrapClusterTestObject(objects)
	if err != nil {
		return config, nil, err
	}

	if !cluster.Spec.Topology.IsDefined() {
		return config, nil, pkgerrors.Errorf("unable to check topology for Cluster %s. support for clusters without spec.topology not implemented yet", klog.KObj(cluster))
	}
	return config, cluster, nil
}

func joinTopologyChecks(checks []TopologyCheckType) string {
	names := make([]string, 0, len(checks))
	for _, check := range checks {
		names = append(names, string(check))
	}
	return strings.Join(names, ", ")
}
