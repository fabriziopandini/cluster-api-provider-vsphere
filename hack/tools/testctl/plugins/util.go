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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	pkgerrors "github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg/desiredstate"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	kubeconfigutil "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getClusters(ctx context.Context, c client.Client, nameRegEx string, limit *int32) ([]*clusterv1.Cluster, error) {
	clusterList := &clusterv1.ClusterList{}
	if err := c.List(ctx, clusterList); err != nil {
		return nil, pkgerrors.Wrap(err, "failed to list Clusters")
	}

	var regex *regexp.Regexp
	if nameRegEx != "" {
		var err error
		regex, err = regexp.Compile(nameRegEx)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to parse nameRegex %s", nameRegEx)
		}
	}

	clusters := make([]*clusterv1.Cluster, 0, len(clusterList.Items))
	for _, cluster := range clusterList.Items {
		if regex != nil && !regex.MatchString(cluster.Name) {
			continue
		}

		clusters = append(clusters, &cluster)
	}

	sort.Slice(clusters, func(i, j int) bool {
		return klog.KObj(clusters[i]).String() < klog.KObj(clusters[j]).String()
	})

	if limit != nil && len(clusters) >= int(ptr.Deref(limit, 0)) {
		return clusters[:int(ptr.Deref(limit, 0))], nil
	}
	return clusters, nil
}

func getClusterClass(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (*clusterv1.ClusterClass, error) {
	clusterClass := &clusterv1.ClusterClass{}
	key := client.ObjectKey{
		Namespace: cluster.Spec.Topology.ClassRef.Namespace,
		Name:      cluster.Spec.Topology.ClassRef.Name,
	}
	if key.Namespace == "" {
		key.Namespace = cluster.Namespace
	}
	if err := c.Get(ctx, key, clusterClass); err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to get ClusterClass %s for Cluster %s", klog.KRef(key.Namespace, key.Name), klog.KObj(cluster))
	}
	return clusterClass, nil
}

func getControlPlane(ctx context.Context, c client.Client, cluster *clusterv1.Cluster) (*controlplanev1.KubeadmControlPlane, error) {
	controlPlane := &controlplanev1.KubeadmControlPlane{}
	key := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Spec.ControlPlaneRef.Name,
	}
	if err := c.Get(ctx, key, controlPlane); err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to get KubeadmControlPlane %s for Cluster %s", klog.KRef(key.Namespace, key.Name), klog.KObj(cluster))
	}
	return controlPlane, nil
}

func getControlPlaneMachines(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, controlPlane *controlplanev1.KubeadmControlPlane) ([]*clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.MachineControlPlaneLabel: "",
			clusterv1.ClusterNameLabel:         cluster.Name,
		},
	}
	if err := c.List(ctx, machineList, listOptions...); err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to list Machines for KubeadmControlPlane %s", klog.KObj(controlPlane))
	}

	machines := []*clusterv1.Machine{}
	for _, machine := range machineList.Items {
		machines = append(machines, &machine)
	}
	return machines, nil
}

func waitForControlPlaneMachines(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, controlPlane *controlplanev1.KubeadmControlPlane, replicas int32, version string, additionalChecks ...machineCheckOption) (done bool, err error, retryErr error) {
	machines, err := getControlPlaneMachines(ctx, c, cluster, controlPlane)
	if err != nil {
		return false, err, nil
	}

	if int32(len(machines)) != replicas {
		return false, nil, pkgerrors.Errorf("waiting for %d KubeadmControlPlane machines to exist, found %d", replicas, len(machines))
	}

	for _, m := range machines {
		if m.Spec.Version != version {
			return false, nil, pkgerrors.Errorf("waiting for %d KubeadmControlPlane machines to have version %s", replicas, version)
		}

		for _, opt := range additionalChecks {
			if err := opt.Check(m); err != nil {
				return false, nil, err
			}
		}

		if !conditions.IsTrue(m, clusterv1.MachineAvailableCondition) {
			return false, nil, pkgerrors.Errorf("waiting for %d KubeadmControlPlane machines to have Available condition True", replicas)
		}

		if conditions.IsTrue(m, clusterv1.MachineUpdatingCondition) {
			return false, nil, pkgerrors.Errorf("waiting for %d KubeadmControlPlane machines to have Updating condition False", replicas)
		}

		if !conditions.IsTrue(m, clusterv1.MachineUpToDateCondition) {
			return false, nil, pkgerrors.Errorf("waiting for %d KubeadmControlPlane machines to have UpToDate condition True", replicas)
		}
	}
	return true, nil, nil
}

type machineCheckOption interface {
	Check(m *clusterv1.Machine) error
}

type MachineMetadataCheck struct {
	topology clusterv1.ObjectMeta
	class    clusterv1.ObjectMeta
}

func newMachineMetadataCheck(topology clusterv1.ObjectMeta, class clusterv1.ObjectMeta) MachineMetadataCheck {
	return MachineMetadataCheck{
		topology: topology,
		class:    class,
	}
}

func (o MachineMetadataCheck) Check(m *clusterv1.Machine) error {
	// Note: Labels and annotations from both class and topology should be applied.
	for k, v := range util.MergeMap(o.topology.Labels, o.class.Labels) {
		if m.Labels[k] != v {
			return pkgerrors.Errorf("waiting for Machine %s to have label %s=%s", klog.KObj(m), k, v)
		}
	}
	for k, v := range util.MergeMap(o.topology.Annotations, o.class.Annotations) {
		if m.Annotations[k] != v {
			return pkgerrors.Errorf("waiting for Machine %s to have annotation %s=%s", klog.KObj(m), k, v)
		}
	}
	return nil
}

type ReadinessGatesCheck struct {
	topology []clusterv1.MachineReadinessGate
	class    []clusterv1.MachineReadinessGate
}

func newReadinessGatesCheck(topology []clusterv1.MachineReadinessGate, class []clusterv1.MachineReadinessGate) ReadinessGatesCheck {
	return ReadinessGatesCheck{
		topology: topology,
		class:    class,
	}
}

func (o ReadinessGatesCheck) Check(m *clusterv1.Machine) error {
	// Note: ReadinessGates should be taken from topology with fallback on class.
	readinessGates := o.topology
	if readinessGates == nil {
		readinessGates = o.class
	}
	var allReadinessGates []clusterv1.MachineReadinessGate
	if util.IsControlPlaneMachine(m) {
		allReadinessGates = append(allReadinessGates, desiredstate.MandatoryMachineReadinessGates...)
		allReadinessGates = append(allReadinessGates,
			clusterv1.MachineReadinessGate{ConditionType: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition},
			clusterv1.MachineReadinessGate{ConditionType: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition},
		)
	}
	allReadinessGates = append(allReadinessGates, readinessGates...)
	if !reflect.DeepEqual(m.Spec.ReadinessGates, allReadinessGates) {
		return pkgerrors.Errorf("waiting for Machine %s to have desired spec.readinessGates", klog.KObj(m))
	}
	return nil
}

type TaintsCheck struct {
	topology []clusterv1.MachineTaint
	class    []clusterv1.MachineTaint
}

func newTaintsCheck(topology []clusterv1.MachineTaint, class []clusterv1.MachineTaint) TaintsCheck {
	return TaintsCheck{
		topology: topology,
		class:    class,
	}
}

func (o TaintsCheck) Check(m *clusterv1.Machine) error {
	// Note: Taints should be taken from topology with fallback on class.
	taints := o.topology
	if taints == nil {
		taints = o.class
	}
	if !reflect.DeepEqual(m.Spec.Taints, taints) {
		return pkgerrors.Errorf("waiting for Machine %s to have desired spec.taints", klog.KObj(m))
	}
	return nil
}

type ControlPlaneDeletionCheck struct {
	topology clusterv1.ControlPlaneTopologyMachineDeletionSpec
	class    clusterv1.ControlPlaneClassMachineDeletionSpec
}

func newControlPlaneDeletionCheck(topology clusterv1.ControlPlaneTopologyMachineDeletionSpec, class clusterv1.ControlPlaneClassMachineDeletionSpec) ControlPlaneDeletionCheck {
	return ControlPlaneDeletionCheck{
		topology: topology,
		class:    class,
	}
}

func (o ControlPlaneDeletionCheck) Check(m *clusterv1.Machine) error {
	// Note: Deletion should be taken from topology with fallback on class.
	// Note: m.Spec.Deletion is a distinct (but field-compatible) type from o.topology, so the
	// fields must be copied into a MachineDeletionSpec value rather than compared directly --
	// reflect.DeepEqual always returns false across distinct declared types.
	deletion := clusterv1.MachineDeletionSpec{
		NodeDrainTimeoutSeconds:        o.topology.NodeDrainTimeoutSeconds,
		NodeVolumeDetachTimeoutSeconds: o.topology.NodeVolumeDetachTimeoutSeconds,
		NodeDeletionTimeoutSeconds:     o.topology.NodeDeletionTimeoutSeconds,
	}
	if deletion.NodeDeletionTimeoutSeconds == nil {
		deletion.NodeDeletionTimeoutSeconds = o.class.NodeDeletionTimeoutSeconds
	}
	if deletion.NodeDeletionTimeoutSeconds == nil {
		deletion.NodeDeletionTimeoutSeconds = new(int32(10))
	}
	if deletion.NodeDrainTimeoutSeconds == nil {
		deletion.NodeDrainTimeoutSeconds = o.class.NodeDrainTimeoutSeconds
	}
	if deletion.NodeVolumeDetachTimeoutSeconds == nil {
		deletion.NodeVolumeDetachTimeoutSeconds = o.class.NodeVolumeDetachTimeoutSeconds
	}
	if !reflect.DeepEqual(m.Spec.Deletion, deletion) {
		return pkgerrors.Errorf("waiting for Machine %s to have desired spec.deletion", klog.KObj(m))
	}
	return nil
}

type MachineDeploymentDeletionCheck struct {
	topology clusterv1.MachineDeploymentTopologyMachineDeletionSpec
	class    clusterv1.MachineDeploymentClassMachineDeletionSpec
}

func newMachineDeploymentDeletionCheck(topology clusterv1.MachineDeploymentTopologyMachineDeletionSpec, class clusterv1.MachineDeploymentClassMachineDeletionSpec) MachineDeploymentDeletionCheck {
	return MachineDeploymentDeletionCheck{
		topology: topology,
		class:    class,
	}
}

func (o MachineDeploymentDeletionCheck) Check(m *clusterv1.Machine) error {
	// Note: Deletion should be taken from topology with fallback on class.
	// Note: Order is not checked here: it only affects which existing Machines a MachineSet
	// chooses to delete on scale down, and is not part of Machine.Spec.Deletion.
	// Note: m.Spec.Deletion is a distinct (but field-compatible) type from o.topology, so the
	// fields must be copied into a MachineDeletionSpec value rather than compared directly --
	// reflect.DeepEqual always returns false across distinct declared types.
	deletion := clusterv1.MachineDeletionSpec{
		NodeDrainTimeoutSeconds:        o.topology.NodeDrainTimeoutSeconds,
		NodeVolumeDetachTimeoutSeconds: o.topology.NodeVolumeDetachTimeoutSeconds,
		NodeDeletionTimeoutSeconds:     o.topology.NodeDeletionTimeoutSeconds,
	}
	if deletion.NodeDeletionTimeoutSeconds == nil {
		deletion.NodeDeletionTimeoutSeconds = o.class.NodeDeletionTimeoutSeconds
	}
	if deletion.NodeDeletionTimeoutSeconds == nil {
		deletion.NodeDeletionTimeoutSeconds = new(int32(10))
	}
	if deletion.NodeDrainTimeoutSeconds == nil {
		deletion.NodeDrainTimeoutSeconds = o.class.NodeDrainTimeoutSeconds
	}
	if deletion.NodeVolumeDetachTimeoutSeconds == nil {
		deletion.NodeVolumeDetachTimeoutSeconds = o.class.NodeVolumeDetachTimeoutSeconds
	}
	if !reflect.DeepEqual(m.Spec.Deletion, deletion) {
		return pkgerrors.Errorf("waiting for Machine %s to have desired spec.deletion", klog.KObj(m))
	}
	return nil
}

type FailureDomainCheck struct {
	topology string
	class    string
}

func newFailureDomainCheck(topology string, class string) FailureDomainCheck {
	return FailureDomainCheck{
		topology: topology,
		class:    class,
	}
}

func (o FailureDomainCheck) Check(m *clusterv1.Machine) error {
	// Note: FailureDomain should be taken from topology with fallback on class.
	failureDomain := o.topology
	if failureDomain == "" {
		failureDomain = o.class
	}
	if !reflect.DeepEqual(m.Spec.FailureDomain, failureDomain) {
		return pkgerrors.Errorf("waiting for Machine %s to have desired spec.failureDomain", klog.KObj(m))
	}
	return nil
}

type MinReadySecondsCheck struct {
	topology *int32
	class    *int32
}

func newMinReadySecondsCheck(topology *int32, class *int32) MinReadySecondsCheck {
	return MinReadySecondsCheck{
		topology: topology,
		class:    class,
	}
}

func (o MinReadySecondsCheck) Check(m *clusterv1.Machine) error {
	// Note: MinReadySeconds should be taken from topology with fallback on class.
	minReadySeconds := o.topology
	if minReadySeconds == nil {
		minReadySeconds = o.class
	}
	if !reflect.DeepEqual(m.Spec.MinReadySeconds, minReadySeconds) {
		return pkgerrors.Errorf("waiting for Machine %s to have desired spec.minReadySeconds", klog.KObj(m))
	}
	return nil
}

func waitForControlPlane(controlPlane *controlplanev1.KubeadmControlPlane, version string, topology *clusterv1.ControlPlaneTopology, class *clusterv1.ControlPlaneClass) (done bool, retryErr error) {
	// Note: Labels and annotations from both class and topology should be applied
	for k, v := range util.MergeMap(topology.Metadata.Labels, class.Metadata.Labels) {
		if controlPlane.Labels[k] != v {
			return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have label %s=%s", klog.KObj(controlPlane), k, v)
		}
	}
	for k, v := range util.MergeMap(topology.Metadata.Annotations, class.Metadata.Annotations) {
		if controlPlane.Annotations[k] != v {
			return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have annotation %s=%s", klog.KObj(controlPlane), k, v)
		}
	}

	if controlPlane.Spec.Version != version {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have desired version", klog.KObj(controlPlane))
	}
	if topology.Replicas != nil && ptr.Deref(controlPlane.Spec.Replicas, 0) != *topology.Replicas {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have desired spec.replicas", klog.KObj(controlPlane))
	}

	// Note: Rollout should be taken from topology (it cannot be defined in class)
	rollout := controlplanev1.KubeadmControlPlaneRolloutSpec{
		After: topology.Rollout.After,
		// Apply defaults (cannot be changed in topology)
		Strategy: controlplanev1.KubeadmControlPlaneRolloutStrategy{
			Type: controlplanev1.RollingUpdateStrategyType,
			RollingUpdate: controlplanev1.KubeadmControlPlaneRolloutStrategyRollingUpdate{
				MaxSurge: intstr.ValueOrDefault(nil, intstr.FromInt32(1)),
			},
		},
	}
	if !reflect.DeepEqual(controlPlane.Spec.Rollout, rollout) {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have desired spec.rollout", klog.KObj(controlPlane))
	}

	// Note: ReadinessGates should be taken from topology with fallback on class.
	readinessGates := topology.ReadinessGates
	if readinessGates == nil {
		readinessGates = class.ReadinessGates
	}
	if !reflect.DeepEqual(controlPlane.Spec.MachineTemplate.Spec.ReadinessGates, readinessGates) {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have desired spec.machineTemplate.spec.readinessGates", klog.KObj(controlPlane))
	}

	// Note: Taints should be taken from topology with fallback on class.
	taints := topology.Taints
	if taints == nil {
		taints = class.Taints
	}
	if !reflect.DeepEqual(controlPlane.Spec.MachineTemplate.Spec.Taints, taints) {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have desired spec.machineTemplate.spec.taints", klog.KObj(controlPlane))
	}

	// Note: Deletion should be taken from topology with fallback on class.
	// Note: controlPlane.Spec.MachineTemplate.Spec.Deletion is a distinct (but field-compatible)
	// type from topology.Deletion, so the fields must be copied into a
	// KubeadmControlPlaneMachineTemplateDeletionSpec value rather than compared directly --
	// reflect.DeepEqual always returns false across distinct declared types.
	deletion := controlplanev1.KubeadmControlPlaneMachineTemplateDeletionSpec{
		NodeDrainTimeoutSeconds:        topology.Deletion.NodeDrainTimeoutSeconds,
		NodeVolumeDetachTimeoutSeconds: topology.Deletion.NodeVolumeDetachTimeoutSeconds,
		NodeDeletionTimeoutSeconds:     topology.Deletion.NodeDeletionTimeoutSeconds,
	}
	if deletion.NodeDeletionTimeoutSeconds == nil {
		deletion.NodeDeletionTimeoutSeconds = class.Deletion.NodeDeletionTimeoutSeconds
	}
	if deletion.NodeDrainTimeoutSeconds == nil {
		deletion.NodeDrainTimeoutSeconds = class.Deletion.NodeDrainTimeoutSeconds
	}
	if deletion.NodeVolumeDetachTimeoutSeconds == nil {
		deletion.NodeVolumeDetachTimeoutSeconds = class.Deletion.NodeVolumeDetachTimeoutSeconds
	}
	if !reflect.DeepEqual(controlPlane.Spec.MachineTemplate.Spec.Deletion, deletion) {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have desired spec.machineTemplate.spec.deletion", klog.KObj(controlPlane))
	}

	// Note:
	// - It is not possible to check if changes to topology.Variables has been propagated, because their impact is defined by patches
	// - Changes to topology.HealthCheck are validated outside if this func

	// Additional checks for control plane stable
	if !conditions.IsTrue(controlPlane, controlplanev1.KubeadmControlPlaneInitializedCondition) {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have Initialized condition True", klog.KObj(controlPlane))
	}
	if !conditions.IsTrue(controlPlane, controlplanev1.KubeadmControlPlaneAvailableCondition) {
		return false, pkgerrors.Errorf("waiting for KubeadmControlPlane %s to have Available condition True", klog.KObj(controlPlane))
	}
	return true, nil
}

func getControlPlaneMachineHealthCheck(ctx context.Context, c client.Client, controlPlane *controlplanev1.KubeadmControlPlane) (*clusterv1.MachineHealthCheck, error) {
	mhc := &clusterv1.MachineHealthCheck{}
	// MachineHealthCheck always has the same name and namespace as the ControlPlane object it belongs to.
	if err := c.Get(ctx, client.ObjectKeyFromObject(controlPlane), mhc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, pkgerrors.Wrapf(err, "failed to get MachineHealthCheck for KubeadmControlPlane %s", klog.KObj(controlPlane))
	}
	return mhc, nil
}

func waitForControlPlaneMachineHealthCheck(ctx context.Context, c client.Client, controlPlane *controlplanev1.KubeadmControlPlane, topology clusterv1.ControlPlaneTopologyHealthCheck, class clusterv1.ControlPlaneClassHealthCheck) (done bool, err error, retryErr error) {
	mhc, err := getControlPlaneMachineHealthCheck(ctx, c, controlPlane)
	if err != nil {
		return false, err, nil
	}

	shouldExist := ptr.Deref(topology.Enabled, true) && (topology.IsDefined() || class.IsDefined())
	if !shouldExist {
		if mhc != nil {
			return false, nil, pkgerrors.Errorf("waiting for the KubeadmControlPlane MachineHealthCheck to be deleted")
		}
		return true, nil, nil
	}

	if mhc == nil {
		return false, nil, pkgerrors.Errorf("waiting for a MachineHealthCheck to be created for the KubeadmControlPlane")
	}

	// Note: MachineHealthCheck should be taken from topology with fallback on class.
	checks := clusterv1.MachineHealthCheckChecks{
		NodeStartupTimeoutSeconds:  topology.Checks.NodeStartupTimeoutSeconds,
		UnhealthyNodeConditions:    topology.Checks.UnhealthyNodeConditions,
		UnhealthyMachineConditions: topology.Checks.UnhealthyMachineConditions,
	}
	remediation := clusterv1.MachineHealthCheckRemediation{
		TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
			UnhealthyLessThanOrEqualTo: topology.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
			UnhealthyInRange:           topology.Remediation.TriggerIf.UnhealthyInRange,
		},
		TemplateRef: topology.Remediation.TemplateRef,
	}
	if !topology.IsDefined() {
		checks = clusterv1.MachineHealthCheckChecks{
			NodeStartupTimeoutSeconds:  class.Checks.NodeStartupTimeoutSeconds,
			UnhealthyNodeConditions:    class.Checks.UnhealthyNodeConditions,
			UnhealthyMachineConditions: class.Checks.UnhealthyMachineConditions,
		}
		remediation = clusterv1.MachineHealthCheckRemediation{
			TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
				UnhealthyLessThanOrEqualTo: class.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
				UnhealthyInRange:           class.Remediation.TriggerIf.UnhealthyInRange,
			},
			TemplateRef: class.Remediation.TemplateRef,
		}
	}

	if !reflect.DeepEqual(mhc.Spec.Checks, checks) {
		return false, nil, pkgerrors.Errorf("waiting for the KubeadmControlPlane MachineHealthCheck %s to have desired spec.checks", klog.KObj(mhc))
	}
	if !reflect.DeepEqual(mhc.Spec.Remediation, remediation) {
		return false, nil, pkgerrors.Errorf("waiting for the KubeadmControlPlane MachineHealthCheck %s to have desired spec.remediation", klog.KObj(mhc))
	}
	return true, nil, nil
}

func getMachineDeployments(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, topologyNameRegEx string, limit *int32) ([]*clusterv1.MachineDeployment, error) {
	machineDeploymentList := &clusterv1.MachineDeploymentList{}

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{clusterv1.ClusterNameLabel: cluster.Name},
	}
	if err := c.List(ctx, machineDeploymentList, listOptions...); err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to list MachineDeployments for Cluster %s", klog.KObj(cluster))
	}

	var regex *regexp.Regexp
	if topologyNameRegEx != "" {
		var err error
		regex, err = regexp.Compile(topologyNameRegEx)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "failed to parse nameRegex %s", topologyNameRegEx)
		}
	}

	machineDeployments := make([]*clusterv1.MachineDeployment, 0, len(machineDeploymentList.Items))
	for _, machineDeployment := range machineDeploymentList.Items {
		if topologyName, ok := machineDeployment.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]; ok && regex != nil {
			if !regex.MatchString(topologyName) {
				continue
			}
		}

		machineDeployments = append(machineDeployments, &machineDeployment)
	}

	sort.Slice(machineDeployments, func(i, j int) bool {
		return klog.KObj(machineDeployments[i]).String() < klog.KObj(machineDeployments[j]).String()
	})

	if limit != nil && len(machineDeployments) >= int(ptr.Deref(limit, 0)) {
		return machineDeployments[:int(ptr.Deref(limit, 0))], nil
	}
	return machineDeployments, nil
}

// findMachineDeploymentClass returns the class matching className in clusterClass.Spec.Workers.MachineDeployments.
func findMachineDeploymentClass(clusterClass *clusterv1.ClusterClass, className string) (*clusterv1.MachineDeploymentClass, error) {
	for i := range clusterClass.Spec.Workers.MachineDeployments {
		if clusterClass.Spec.Workers.MachineDeployments[i].Class == className {
			return &clusterClass.Spec.Workers.MachineDeployments[i], nil
		}
	}
	return nil, pkgerrors.Errorf("cannot find a MachineDeployment class with name %s in clusterClass.spec.workers.machineDeployments", className)
}

func getMachineDeployment(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, mdTopologyName string) (*clusterv1.MachineDeployment, error) {
	machineDeploymentList := &clusterv1.MachineDeploymentList{}
	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopologyName,
		},
	}
	if err := c.List(ctx, machineDeploymentList, listOptions...); err != nil {
		return nil, err
	}
	if len(machineDeploymentList.Items) == 1 {
		return &machineDeploymentList.Items[0], nil
	}
	if len(machineDeploymentList.Items) > 1 {
		return nil, pkgerrors.Errorf("there is more than one MachineDeployment with the %s=%s label", clusterv1.ClusterTopologyMachineDeploymentNameLabel, mdTopologyName)
	}
	return nil, nil
}

func getMachineDeploymentMachines(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, machineDeployment *clusterv1.MachineDeployment) ([]*clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}

	listOptions := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: cluster.Name,
		},
		client.MatchingLabels(machineDeployment.Spec.Selector.MatchLabels),
	}
	if err := c.List(ctx, machineList, listOptions...); err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to list Machines for MachineDeployment %s", klog.KObj(machineDeployment))
	}

	machines := []*clusterv1.Machine{}
	for _, machine := range machineList.Items {
		machines = append(machines, &machine)
	}
	return machines, nil
}

func waitForMachineDeploymentMachines(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, machineDeployment *clusterv1.MachineDeployment, replicas int32, version string, additionalChecks ...machineCheckOption) (done bool, err error, retryErr error) {
	machines, err := getMachineDeploymentMachines(ctx, c, cluster, machineDeployment)
	if err != nil {
		return false, err, nil
	}

	if int32(len(machines)) != replicas {
		return false, nil, pkgerrors.Errorf("waiting for %d MachineDeployment machines to exist, found %d", replicas, len(machines))
	}

	for _, m := range machines {
		if m.Spec.Version != version {
			return false, nil, pkgerrors.Errorf("waiting for %d KubeadmControlPlane machines to have version %s", replicas, version)
		}

		if !conditions.IsTrue(m, clusterv1.MachineAvailableCondition) {
			return false, nil, pkgerrors.Errorf("waiting for %d MachineDeployment machines to have Available condition True", replicas)
		}

		if conditions.IsTrue(m, clusterv1.MachineUpdatingCondition) {
			return false, nil, pkgerrors.Errorf("waiting for %d MachineDeployment machines to have Updating condition False", replicas)
		}

		if !conditions.IsTrue(m, clusterv1.MachineUpToDateCondition) {
			return false, nil, pkgerrors.Errorf("waiting for %d MachineDeployment machines to have UpToDate condition True", replicas)
		}

		for _, opt := range additionalChecks {
			if err := opt.Check(m); err != nil {
				return false, nil, err
			}
		}
	}
	return true, nil, nil
}

func waitForMachineDeployment(machineDeployment *clusterv1.MachineDeployment, version string, topology *clusterv1.MachineDeploymentTopology, class *clusterv1.MachineDeploymentClass) (done bool, retryErr error) {
	// Note: Labels and annotations from both class and topology should be applied
	for k, v := range util.MergeMap(topology.Metadata.Labels, class.Metadata.Labels) {
		if machineDeployment.Labels[k] != v {
			return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have label %s=%s", klog.KObj(machineDeployment), k, v)
		}
	}
	for k, v := range util.MergeMap(topology.Metadata.Annotations, class.Metadata.Annotations) {
		if machineDeployment.Annotations[k] != v {
			return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have annotation %s=%s", klog.KObj(machineDeployment), k, v)
		}
	}

	if machineDeployment.Spec.Template.Spec.Version != version {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired version", klog.KObj(machineDeployment))
	}
	if topology.Replicas != nil && ptr.Deref(machineDeployment.Spec.Replicas, 0) != *topology.Replicas {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.replicas", klog.KObj(machineDeployment))
	}

	// Note: MinReadySeconds should be taken from topology with fallback on class.
	minReadySeconds := topology.MinReadySeconds
	if minReadySeconds == nil {
		minReadySeconds = class.MinReadySeconds
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Template.Spec.MinReadySeconds, minReadySeconds) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.template.spec.minReadySeconds", klog.KObj(machineDeployment))
	}

	// Note: Rollout should be taken from topology with fallback on class.
	rollout := clusterv1.MachineDeploymentRolloutSpec{
		After: metav1.Time{},
		Strategy: clusterv1.MachineDeploymentRolloutStrategy{
			Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
			RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
				MaxUnavailable: new(intstr.FromInt32(0)),
				MaxSurge:       new(intstr.FromInt32(1)),
			},
		},
	}

	if !reflect.DeepEqual(class.Rollout, clusterv1.MachineDeploymentClassRolloutSpec{}) {
		rollout = clusterv1.MachineDeploymentRolloutSpec{
			Strategy: clusterv1.MachineDeploymentRolloutStrategy{
				Type: class.Rollout.Strategy.Type,
				RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
					MaxUnavailable: class.Rollout.Strategy.RollingUpdate.MaxUnavailable,
					MaxSurge:       class.Rollout.Strategy.RollingUpdate.MaxSurge,
				},
			},
		}
	}
	if !reflect.DeepEqual(topology.Rollout, clusterv1.MachineDeploymentTopologyRolloutSpec{}) {
		rollout = clusterv1.MachineDeploymentRolloutSpec{
			After: topology.Rollout.After,
			Strategy: clusterv1.MachineDeploymentRolloutStrategy{
				Type: topology.Rollout.Strategy.Type,
				RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
					MaxUnavailable: topology.Rollout.Strategy.RollingUpdate.MaxUnavailable,
					MaxSurge:       topology.Rollout.Strategy.RollingUpdate.MaxSurge,
				},
			},
		}
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Rollout, rollout) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.rollout", klog.KObj(machineDeployment))
	}

	// Note: Remediation should be taken from topology with fallback on class.
	remediation := clusterv1.MachineDeploymentRemediationSpec{}
	remediation.MaxInFlight = topology.HealthCheck.Remediation.MaxInFlight
	if remediation.MaxInFlight == nil {
		remediation.MaxInFlight = class.HealthCheck.Remediation.MaxInFlight
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Remediation, remediation) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.remediation", klog.KObj(machineDeployment))
	}

	// Note: FailureDomain should be taken from topology with fallback on class.
	failureDomain := topology.FailureDomain
	if failureDomain == "" {
		failureDomain = class.FailureDomain
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Template.Spec.FailureDomain, failureDomain) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.template.spec.failureDomain", klog.KObj(machineDeployment))
	}

	// Note: ReadinessGates should be taken from topology with fallback on class.
	readinessGates := topology.ReadinessGates
	if readinessGates == nil {
		readinessGates = class.ReadinessGates
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Template.Spec.ReadinessGates, readinessGates) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.template.spec.readinessGates", klog.KObj(machineDeployment))
	}

	// Note: Taints should be taken from topology with fallback on class.
	taints := topology.Taints
	if taints == nil {
		taints = class.Taints
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Template.Spec.Taints, taints) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.template.spec.taints", klog.KObj(machineDeployment))
	}

	// Note: Deletion.Order should be taken from topology with fallback on class.
	order := topology.Deletion.Order
	if order == "" {
		order = class.Deletion.Order
	}
	if machineDeployment.Spec.Deletion.Order != order {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.deletion.order", klog.KObj(machineDeployment))
	}

	// Note: the remaining Deletion fields should be taken from topology with fallback on class.
	deletion := clusterv1.MachineDeletionSpec{
		NodeDrainTimeoutSeconds:        topology.Deletion.NodeDrainTimeoutSeconds,
		NodeVolumeDetachTimeoutSeconds: topology.Deletion.NodeVolumeDetachTimeoutSeconds,
		NodeDeletionTimeoutSeconds:     topology.Deletion.NodeDeletionTimeoutSeconds,
	}
	if deletion.NodeDeletionTimeoutSeconds == nil {
		deletion.NodeDeletionTimeoutSeconds = class.Deletion.NodeDeletionTimeoutSeconds
	}
	if deletion.NodeDrainTimeoutSeconds == nil {
		deletion.NodeDrainTimeoutSeconds = class.Deletion.NodeDrainTimeoutSeconds
	}
	if deletion.NodeVolumeDetachTimeoutSeconds == nil {
		deletion.NodeVolumeDetachTimeoutSeconds = class.Deletion.NodeVolumeDetachTimeoutSeconds
	}
	if !reflect.DeepEqual(machineDeployment.Spec.Template.Spec.Deletion, deletion) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have desired spec.template.spec.deletion", klog.KObj(machineDeployment))
	}

	// Note:
	// - It is not possible to check if changes to topology.Variables has been propagated, because their impact is defined by patches
	// - Changes to topology.HealthCheck are validated outside if this func

	// Additional checks for control plane stable
	if !conditions.IsTrue(machineDeployment, clusterv1.MachineDeploymentAvailableCondition) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to have Available condition True", klog.KObj(machineDeployment))
	}
	if conditions.IsTrue(machineDeployment, clusterv1.MachineDeploymentRollingOutCondition) {
		return false, pkgerrors.Errorf("waiting for MachineDeployment %s to not be RollingOut", klog.KObj(machineDeployment))
	}
	return true, nil
}

func getMachineDeploymentMachineHealthCheck(ctx context.Context, c client.Client, machineDeployment *clusterv1.MachineDeployment) (*clusterv1.MachineHealthCheck, error) {
	mhc := &clusterv1.MachineHealthCheck{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(machineDeployment), mhc); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, pkgerrors.Wrapf(err, "failed to get MachineHealthCheck for MachineDeployment %s", klog.KObj(machineDeployment))
	}
	return mhc, nil
}

func waitForMachineDeploymentMachineHealthCheck(ctx context.Context, c client.Client, machineDeployment *clusterv1.MachineDeployment, topology clusterv1.MachineDeploymentTopologyHealthCheck, class clusterv1.MachineDeploymentClassHealthCheck) (done bool, err error, retryErr error) {
	mhc, err := getMachineDeploymentMachineHealthCheck(ctx, c, machineDeployment)
	if err != nil {
		return false, err, nil
	}

	shouldExist := ptr.Deref(topology.Enabled, true) && (topology.IsDefined() || class.IsDefined())
	if !shouldExist {
		if mhc != nil {
			return false, nil, pkgerrors.Errorf("waiting for the MachineDeployment MachineHealthCheck to be deleted")
		}
		return true, nil, nil
	}

	if mhc == nil {
		return false, nil, pkgerrors.Errorf("waiting for a MachineHealthCheck to be created for the MachineDeployment")
	}

	// Note: MachineHealthCheck should be taken from topology with fallback on class.
	checks := clusterv1.MachineHealthCheckChecks{
		NodeStartupTimeoutSeconds:  topology.Checks.NodeStartupTimeoutSeconds,
		UnhealthyNodeConditions:    topology.Checks.UnhealthyNodeConditions,
		UnhealthyMachineConditions: topology.Checks.UnhealthyMachineConditions,
	}
	remediation := clusterv1.MachineHealthCheckRemediation{
		TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
			UnhealthyLessThanOrEqualTo: topology.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
			UnhealthyInRange:           topology.Remediation.TriggerIf.UnhealthyInRange,
		},
		TemplateRef: topology.Remediation.TemplateRef,
	}
	if !topology.IsDefined() {
		checks = clusterv1.MachineHealthCheckChecks{
			NodeStartupTimeoutSeconds:  class.Checks.NodeStartupTimeoutSeconds,
			UnhealthyNodeConditions:    class.Checks.UnhealthyNodeConditions,
			UnhealthyMachineConditions: class.Checks.UnhealthyMachineConditions,
		}
		remediation = clusterv1.MachineHealthCheckRemediation{
			TriggerIf: clusterv1.MachineHealthCheckRemediationTriggerIf{
				UnhealthyLessThanOrEqualTo: class.Remediation.TriggerIf.UnhealthyLessThanOrEqualTo,
				UnhealthyInRange:           class.Remediation.TriggerIf.UnhealthyInRange,
			},
			TemplateRef: class.Remediation.TemplateRef,
		}
	}

	if !reflect.DeepEqual(mhc.Spec.Checks, checks) {
		return false, nil, pkgerrors.Errorf("waiting for the MachineDeployment MachineHealthCheck %s to have desired spec.checks", klog.KObj(mhc))
	}
	if !reflect.DeepEqual(mhc.Spec.Remediation, remediation) {
		return false, nil, pkgerrors.Errorf("waiting for the MachineDeployment MachineHealthCheck %s to have desired spec.remediation", klog.KObj(mhc))
	}
	return true, nil, nil
}

// resolveKubeconfig returns the kubeconfig to be used by an external CLI invoked by a plugin,
// together with a human-readable name for the target cluster (used only for logging/messages).
//
// If objects carries a Cluster (i.e. the plugin is nested under select.cluster.testctl), the
// workload cluster's kubeconfig is returned; otherwise the management cluster's kubeconfig is
// returned.
func resolveKubeconfig(ctx context.Context, c client.Client, objects core.TestObjects) (kubeconfig []byte, target string, err error) {
	clusterObj, ok := objects[ClusterObjectKey]
	if !ok {
		kubeconfigObj, ok := objects[core.ManagementKubeConfigKey]
		if !ok {
			return nil, "", pkgerrors.New("management cluster kubeconfig is not available")
		}

		kubeconfig, ok = kubeconfigObj.([]byte)
		if !ok {
			return nil, "", pkgerrors.Errorf("failed to cast the %s object to the target type", core.ManagementKubeConfigKey)
		}

		return kubeconfig, "management", nil
	}

	cluster, ok := clusterObj.(*clusterv1.Cluster)
	if !ok {
		return nil, "", pkgerrors.Errorf("failed to cast the %s object to the target type", ClusterObjectKey)
	}

	kubeconfig, err = kubeconfigutil.FromSecret(ctx, c, client.ObjectKeyFromObject(cluster))
	if err != nil {
		return nil, "", pkgerrors.Wrapf(err, "failed to get kubeconfig for workload Cluster %s", klog.KObj(cluster))
	}
	return kubeconfig, "workload", nil
}

// writeTempKubeconfig writes kubeconfig to a temp file external CLIs can be pointed at via
// --kubeconfig, returning a cleanup func to remove it.
func writeTempKubeconfig(kubeconfig []byte) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "testctl-kubeconfig-*")
	if err != nil {
		return "", nil, pkgerrors.Wrap(err, "failed to create temp kubeconfig file")
	}
	cleanup = func() { _ = os.Remove(f.Name()) }

	if err := f.Chmod(0o600); err != nil {
		cleanup()
		return "", nil, pkgerrors.Wrap(err, "failed to set permissions on temp kubeconfig file")
	}
	if _, err := f.Write(kubeconfig); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, pkgerrors.Wrap(err, "failed to write temp kubeconfig file")
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, pkgerrors.Wrap(err, "failed to close temp kubeconfig file")
	}
	return f.Name(), cleanup, nil
}

// lookupBinary resolves the path to an external binary: envVar overrides the default lookup,
// otherwise defaultName is resolved via the process PATH.
func lookupBinary(envVar, defaultName string) (string, error) {
	if override := os.Getenv(envVar); override != "" {
		return override, nil
	}
	path, err := exec.LookPath(defaultName)
	if err != nil {
		return "", pkgerrors.Errorf("%s binary not found on PATH; install %s or set %s", defaultName, defaultName, envVar)
	}
	return path, nil
}

// runExternalCommand runs binPath with args, appending extraEnv to the process environment and
// feeding stdin (if not nil) to the command's standard input. On failure, the returned error is
// annotated with the command's stderr output.
func runExternalCommand(ctx context.Context, log logr.Logger, binPath string, args []string, extraEnv []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binPath, args...) //nolint:gosec // binPath/args are built from testctl's own config, not from an untrusted external source
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	log.V(2).Info(fmt.Sprintf("Running command: %s %s", binPath, strings.Join(args, " ")))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to run %s %s: %s", binPath, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// countManifestDocuments counts the non-empty YAML documents in a multi-document manifest.
func countManifestDocuments(manifest []byte) int {
	docs := strings.Split(string(manifest), "\n---")
	count := 0
	for _, doc := range docs {
		if strings.TrimSpace(doc) != "" {
			count++
		}
	}
	return count
}
