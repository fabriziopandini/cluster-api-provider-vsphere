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

	pkgerrors "github.com/pkg/errors"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const helmChartUninstallPluginKey = "uninstall.helmChart.testctl"

// HelmChartUninstallPlugin can be used to uninstall a Helm release, from either the management
// cluster or the workload cluster of a selected Cluster.
type HelmChartUninstallPlugin struct{}

var _ core.ExecutorPlugin = &HelmChartUninstallPlugin{}
var _ core.ConfigurablePlugin = &HelmChartUninstallPlugin{}
var _ core.MessageGeneratorPlugin = &HelmChartUninstallPlugin{}

// HelmChartUninstallPluginConfig defines the config for the HelmChartUninstallPlugin.
type HelmChartUninstallPluginConfig struct {
	// ReleaseName is the Helm release name.
	ReleaseName string `json:"releaseName"`

	// Namespace is the namespace of the release. If not set, it defaults to "default".
	Namespace string `json:"namespace,omitempty"`
}

func init() {
	registerPlugin(helmChartUninstallPluginKey, &HelmChartUninstallPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p HelmChartUninstallPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
	config := p.unwrapConfig(pluginConfigUntyped)

	kubeconfig, target, err := resolveKubeconfig(ctx, c, objects)
	if err != nil {
		return err
	}

	kubeconfigPath, cleanup, err := writeTempKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	defer cleanup()

	log := ctrl.LoggerFrom(ctx).WithValues("Release", config.ReleaseName, "Namespace", p.namespace(config), "Target", target)
	ctx = ctrl.LoggerInto(ctx, log)

	helmBin, err := lookupBinary("TESTCTL_HELM_BIN", "helm")
	if err != nil {
		return err
	}

	exists, err := helmReleaseExists(ctx, helmBin, kubeconfigPath, p.namespace(config), config.ReleaseName)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to check if release %s exists", config.ReleaseName)
	}
	if !exists {
		log.Info("Release does not exist, skipping")
		return nil
	}

	args := buildHelmUninstallArgs(config, kubeconfigPath, runConfig)

	log.Info("Uninstalling Helm release")
	if _, err := runExternalCommand(ctx, log, helmBin, args, nil, nil); err != nil {
		return pkgerrors.Wrapf(err, "failed to uninstall Helm release %s", config.ReleaseName)
	}

	log.Info("Uninstalled Helm release")
	return nil
}

// buildHelmUninstallArgs builds the argument list for `helm uninstall`.
func buildHelmUninstallArgs(config *HelmChartUninstallPluginConfig, kubeconfigPath string, runConfig core.RunConfig) []string {
	namespace := config.Namespace
	if namespace == "" {
		namespace = "default"
	}

	args := []string{
		"uninstall", config.ReleaseName,
		"--namespace", namespace,
		"--kubeconfig", kubeconfigPath,
	}

	if ptr.Deref(runConfig.DryRun, false) {
		args = append(args, "--dry-run=client")
	}

	if !ptr.Deref(runConfig.SkipWait, false) {
		args = append(args, "--wait")
		if runConfig.Timeout != nil {
			args = append(args, "--timeout", runConfig.Timeout.Duration.String())
		}
	}

	return args
}

// ParseConfig parses the config for this plugin.
func (p HelmChartUninstallPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &HelmChartUninstallPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}

	if config.ReleaseName == "" {
		return nil, pkgerrors.New("releaseName must be set")
	}

	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p HelmChartUninstallPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config := p.unwrapConfig(pluginConfigUntyped)

	return fmt.Sprintf("Uninstall Helm release %s (namespace %s) from the %s cluster",
		config.ReleaseName, p.namespace(config), targetLabel(objects)), nil
}

func (p HelmChartUninstallPlugin) unwrapConfig(pluginConfigUntyped any) *HelmChartUninstallPluginConfig {
	config := &HelmChartUninstallPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*HelmChartUninstallPluginConfig)
	}
	return config
}

func (p HelmChartUninstallPlugin) namespace(config *HelmChartUninstallPluginConfig) string {
	if config.Namespace == "" {
		return "default"
	}
	return config.Namespace
}
