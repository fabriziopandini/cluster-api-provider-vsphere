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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

const manifestApplyPluginKey = "apply.manifest.testctl"

// ManifestApplyPlugin can be used to apply a YAML manifest, substituted with envsubst, on either
// the management cluster or the workload cluster of a selected Cluster. It skips entirely if
// the manifest's objects already exist.
type ManifestApplyPlugin struct {
	manifestResourcesExistOverride func(ctx context.Context, kubectlBin, kubeconfigPath string, manifest []byte) (bool, error)
}

var _ core.ExecutorPlugin = &ManifestApplyPlugin{}
var _ core.ConfigurablePlugin = &ManifestApplyPlugin{}
var _ core.MessageGeneratorPlugin = &ManifestApplyPlugin{}

// ManifestApplyPluginConfig defines the config for the ManifestApplyPlugin.
type ManifestApplyPluginConfig struct {
	// Path is the path to the YAML manifest file to apply.
	Path string `json:"path"`

	// Env is a list of extra variables made available to envsubst, in addition to the process
	// environment; entries here take precedence over the process environment on conflict.
	Env []NameValue `json:"env,omitempty"`
}

// NameValue is a name/value pair used for envsubst variables.
type NameValue struct {
	// Name is the variable name.
	Name string `json:"name"`

	// Value is the variable value.
	Value string `json:"value"`
}

func init() {
	registerPlugin(manifestApplyPluginKey, &ManifestApplyPlugin{})
}

// Exec executes the plugin for the given TestObjects.
func (p ManifestApplyPlugin) Exec(ctx context.Context, c client.Client, objects core.TestObjects, pluginConfigUntyped any, runConfig core.RunConfig) error {
	config := p.unwrapConfig(pluginConfigUntyped)

	log := ctrl.LoggerFrom(ctx).WithValues("Manifest", config.Path, "Target", targetLabel(objects))
	ctx = ctrl.LoggerInto(ctx, log)

	if _, err := os.Stat(config.Path); pkgerrors.Is(err, os.ErrNotExist) {
		return pkgerrors.Errorf("file %s does not exist", config.Path)
	}
	rawManifest, err := os.ReadFile(config.Path)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to read file %s", config.Path)
	}

	envsubstBin, err := lookupBinary("TESTCTL_ENVSUBST_BIN", "envsubst")
	if err != nil {
		return err
	}

	extraEnv := make([]string, 0, len(config.Env))
	for _, nv := range config.Env {
		extraEnv = append(extraEnv, fmt.Sprintf("%s=%s", nv.Name, nv.Value))
	}

	manifest, err := runExternalCommand(ctx, log, envsubstBin, nil, extraEnv, rawManifest)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to substitute variables in manifest %s", config.Path)
	}

	kubeconfig, _, err := resolveKubeconfig(ctx, c, objects)
	if err != nil {
		return err
	}

	kubeconfigPath, cleanup, err := writeTempKubeconfig(kubeconfig)
	if err != nil {
		return err
	}
	defer cleanup()

	kubectlBin, err := lookupBinary("TESTCTL_KUBECTL_BIN", "kubectl")
	if err != nil {
		return err
	}

	manifestResourcesExistFunc := p.manifestResourcesExist
	if p.manifestResourcesExistOverride != nil {
		manifestResourcesExistFunc = p.manifestResourcesExistOverride
	}
	exists, err := manifestResourcesExistFunc(ctx, kubectlBin, kubeconfigPath, manifest)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to check if manifest %s objects already exist", config.Path)
	}
	if exists {
		log.Info("Manifest objects already exist, skipping")
		return nil
	}

	args := buildKubectlApplyArgs(kubeconfigPath, runConfig)

	log.Info("Applying manifest")
	if _, err := runExternalCommand(ctx, log, kubectlBin, args, nil, manifest); err != nil {
		return pkgerrors.Wrapf(err, "failed to apply manifest %s", config.Path)
	}

	log.Info("Applied manifest")
	return nil
}

// buildKubectlApplyArgs builds the argument list for `kubectl apply -f -`.
func buildKubectlApplyArgs(kubeconfigPath string, runConfig core.RunConfig) []string {
	args := []string{"apply", "-f", "-", "--kubeconfig", kubeconfigPath}
	if ptr.Deref(runConfig.DryRun, false) {
		args = append(args, "--dry-run=client")
	}
	return args
}

// ParseConfig parses the config for this plugin.
func (p ManifestApplyPlugin) ParseConfig(_ context.Context, rawPluginConfig []byte) (any, error) {
	config := &ManifestApplyPluginConfig{}
	if err := yaml.UnmarshalStrict(rawPluginConfig, config); err != nil {
		return nil, err
	}

	if config.Path == "" {
		return nil, pkgerrors.New("path must be set")
	}

	return config, nil
}

// GenerateMessage generates a message text for the plugin call.
func (p ManifestApplyPlugin) GenerateMessage(objects core.TestObjects, pluginConfigUntyped any) (string, error) {
	config := p.unwrapConfig(pluginConfigUntyped)

	return fmt.Sprintf("Apply manifest %s on the %s cluster", config.Path, targetLabel(objects)), nil
}

func (p ManifestApplyPlugin) unwrapConfig(pluginConfigUntyped any) *ManifestApplyPluginConfig {
	config := &ManifestApplyPluginConfig{}
	if pluginConfigUntyped != nil {
		config = pluginConfigUntyped.(*ManifestApplyPluginConfig)
	}
	return config
}

func (p ManifestApplyPlugin) manifestResourcesExist(ctx context.Context, kubectlBin, kubeconfigPath string, manifest []byte) (bool, error) {
	log := ctrl.LoggerFrom(ctx)
	out, err := runExternalCommand(ctx, log, kubectlBin,
		[]string{"get", "-f", "-", "--kubeconfig", kubeconfigPath, "--ignore-not-found", "-o", "name"},
		nil, manifest)
	if err != nil {
		return false, err
	}

	found := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			found++
		}
	}
	return found == countManifestDocuments(manifest), nil
}
