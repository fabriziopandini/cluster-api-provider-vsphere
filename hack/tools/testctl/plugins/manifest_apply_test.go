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
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

func Test_buildKubectlApplyArgs(t *testing.T) {
	g := NewWithT(t)

	args := buildKubectlApplyArgs("/tmp/kubeconfig", core.RunConfig{})
	g.Expect(args).To(Equal([]string{"apply", "-f", "-", "--kubeconfig", "/tmp/kubeconfig"}))

	args = buildKubectlApplyArgs("/tmp/kubeconfig", core.RunConfig{DryRun: ptr.To(true)})
	g.Expect(args).To(ContainElement("--dry-run=client"))
}

func Test_ManifestApplyPlugin_Exec_SkipsIfResourcesExist(t *testing.T) {
	g := NewWithT(t)

	objects := core.TestObjects{core.ManagementKubeConfigKey: []byte("management-kubeconfig")}

	manifestPath := filepath.Join(t.TempDir(), "manifest.yaml")
	g.Expect(os.WriteFile(manifestPath, []byte("kind: ConfigMap\n"), 0o600)).To(Succeed())

	p := ManifestApplyPlugin{
		manifestResourcesExistOverride: func(_ context.Context, _, _ string, _ []byte) (bool, error) {
			return true, nil
		},
	}
	config := &ManifestApplyPluginConfig{
		Path: manifestPath,
	}

	c := fake.NewClientBuilder().Build()
	err := p.Exec(t.Context(), c, objects, config, core.RunConfig{})
	g.Expect(err).NotTo(HaveOccurred())
}

func Test_ManifestApplyPlugin_ParseConfig(t *testing.T) {
	g := NewWithT(t)

	p := ManifestApplyPlugin{}

	t.Run("requires path", func(_ *testing.T) {
		_, err := p.ParseConfig(t.Context(), []byte(`{}`))
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("parses a minimal config", func(_ *testing.T) {
		cfg, err := p.ParseConfig(t.Context(), []byte(`{"path": "/tmp/manifest.yaml"}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.(*ManifestApplyPluginConfig).Path).To(Equal("/tmp/manifest.yaml"))
	})
}
