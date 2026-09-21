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
	"os"
	"strings"

	pkgerrors "github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const helmChartInstallPluginKey = "install.helmChart.testctl"

// HelmChartInstallPlugin can be used to install/upgrade a Helm chart, on either the management
// cluster or the workload cluster of a selected Cluster. Existing releases are skipped unless
// Upgrade is enabled.
type HelmChartInstallPlugin struct{}

var _ core.ExecutorPlugin = &HelmChartInstallPlugin{}
var _ core.ConfigurablePlugin = &HelmChartInstallPlugin{}
var _ core.MessageGeneratorPlugin = &HelmChartInstallPlugin{}

// HelmChartInstallPluginConfig defines the config for the HelmChartInstallPlugin.
type HelmChartInstallPluginConfig struct {
	// ReleaseName is the Helm release name.
	ReleaseName string `json:"releaseName"`

	// Chart is the chart reference passed to `helm upgrade --install` (local path, OCI
	// reference, or repo chart name).
	Chart string `json:"chart"`

	// Upgrade allows upgrading an existing release. If false, existing releases are skipped.
	Upgrade bool `json:"upgrade,omitempty"`

	// RepoURL is the Helm repository URL, if Chart is a chart name rather than a local path or
	// OCI reference.
	RepoURL string `json:"repoURL,omitempty"`

	// Version is the chart version to install or upgrade to.
	Version string `json:"version,omitempty"`

	// Namespace is the target namespace for the release. If not set, it defaults to "default".
	Namespace string `json:"namespace,omitempty"`

	// Values is an arbitrary inline block of chart values, using the same shape as
	// https://fluxcd.io/flux/components/helm/helmreleases/#values.
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// Insecure allows connecting to an OCI registry over plain HTTP instead of HTTPS.
	// Maps to `helm`'s --plain-http flag.
	Insecure bool `json:"insecure,omitempty"`
}

// helmReleaseExists checks whether releaseName already exists in namespace. It is a package
// var so it can be stubbed out in tests.
var helmReleaseExists = func(ctx context.Context, helmBin, kubeconfigPath, namespace, releaseName string) (bool, error) {
	args := []string{"status", releaseName, "--kubeconfig", kubeconfigPath}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}

	log := ctrl.LoggerFrom(ctx)
	if _, err := runExternalCommand(ctx, log, helmBin, args, nil, nil); err != nil {
		if strings.Contains(err.Error(), "release: not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func init() {
	registerPlugin(helmChartInstallPluginKey, &HelmChartInstallPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p HelmChartInstallPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
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

	if !config.Upgrade {
		exists, err := helmReleaseExists(ctx, helmBin, kubeconfigPath, p.namespace(config), config.ReleaseName)
		if err != nil {
			return pkgerrors.Wrapf(err, "failed to check if release %s already exists", config.ReleaseName)
		}
		if exists {
			log.Info("Release already exists, skipping")
			return nil
		}
	}

	var valuesPath string
	if config.Values != nil {
		valuesFile, valuesCleanup, err := p.writeTempValues(config.Values.Raw)
		if err != nil {
			return err
		}
		defer valuesCleanup()
		valuesPath = valuesFile
	}

	args := buildHelmArgs(config, kubeconfigPath, valuesPath, runConfig)

	log.Info("Installing or upgrading Helm chart")
	if _, err := runExternalCommand(ctx, log, helmBin, args, nil, nil); err != nil {
		return pkgerrors.Wrapf(err, "failed to install or upgrade Helm chart %s as release %s", config.Chart, config.ReleaseName)
	}

	log.Info("Installed or upgraded Helm chart")
	return nil
}

// buildHelmArgs builds the argument list for `helm upgrade --install`.
func buildHelmArgs(config *HelmChartInstallPluginConfig, kubeconfigPath, valuesPath string, runConfig core.RunConfig) []string {
	namespace := config.Namespace
	if namespace == "" {
		namespace = "default"
	}

	args := []string{
		"upgrade", "--install", config.ReleaseName, config.Chart,
		"--namespace", namespace,
		"--create-namespace",
		"--kubeconfig", kubeconfigPath,
	}

	if config.RepoURL != "" {
		args = append(args, "--repo", config.RepoURL)
	}
	if config.Version != "" {
		args = append(args, "--version", config.Version)
	}
	if valuesPath != "" {
		args = append(args, "-f", valuesPath)
	}
	if config.Insecure {
		args = append(args, "--plain-http")
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
func (p HelmChartInstallPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &HelmChartInstallPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}

	if config.ReleaseName == "" {
		return nil, pkgerrors.New("releaseName must be set")
	}
	if config.Chart == "" {
		return nil, pkgerrors.New("chart must be set")
	}

	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p HelmChartInstallPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config := p.unwrapConfig(pluginConfigUntyped)

	return fmt.Sprintf("Install or upgrade Helm chart %s as release %s (namespace %s) on the %s cluster",
		config.Chart, config.ReleaseName, p.namespace(config), targetLabel(objects)), nil
}

func (p HelmChartInstallPlugin) unwrapConfig(pluginConfigUntyped any) *HelmChartInstallPluginConfig {
	config := &HelmChartInstallPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*HelmChartInstallPluginConfig)
	}
	return config
}

func (p HelmChartInstallPlugin) namespace(config *HelmChartInstallPluginConfig) string {
	if config.Namespace == "" {
		return "default"
	}
	return config.Namespace
}

// writeTempValues writes raw values (JSON, which is also valid YAML) to a temp file for `helm
// -f` to consume.
func (p HelmChartInstallPlugin) writeTempValues(raw []byte) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "testctl-helm-values-*.yaml")
	if err != nil {
		return "", nil, pkgerrors.Wrap(err, "failed to create temp values file")
	}
	cleanup = func() { _ = os.Remove(f.Name()) }

	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, pkgerrors.Wrap(err, "failed to write temp values file")
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, pkgerrors.Wrap(err, "failed to close temp values file")
	}
	return f.Name(), cleanup, nil
}

// targetLabel returns a human-readable name for the cluster a plugin will target, without
// performing any I/O — used for GenerateMessage, where resolveKubeconfig's Secret lookup would
// be an unwanted side effect.
func targetLabel(objects core.TestObjects) string {
	if _, ok := objects[ClusterObjectKey]; ok {
		return "workload"
	}
	return "management"
}
