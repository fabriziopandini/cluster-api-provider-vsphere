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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

func Test_buildHelmArgs(t *testing.T) {
	tests := []struct {
		name           string
		config         *HelmChartInstallPluginConfig
		valuesPath     string
		runConfig      core.RunConfig
		wantArgsSubset []string
		wantArgsAbsent []string
	}{
		{
			name: "minimal config waits by default",
			config: &HelmChartInstallPluginConfig{
				ReleaseName: "my-release",
				Chart:       "my-chart",
			},
			wantArgsSubset: []string{"upgrade", "--install", "my-release", "my-chart", "--namespace", "default", "--create-namespace", "--wait"},
			wantArgsAbsent: []string{"--repo", "--version", "-f", "--dry-run=client", "--timeout"},
		},
		{
			name: "full config",
			config: &HelmChartInstallPluginConfig{
				ReleaseName: "my-release",
				Chart:       "my-chart",
				RepoURL:     "https://example.com/charts",
				Version:     "1.2.3",
				Namespace:   "my-ns",
			},
			valuesPath:     "/tmp/values.yaml",
			runConfig:      core.RunConfig{DryRun: ptr.To(true), Timeout: &metav1.Duration{Duration: 5 * time.Minute}},
			wantArgsSubset: []string{"--repo", "https://example.com/charts", "--version", "1.2.3", "--namespace", "my-ns", "-f", "/tmp/values.yaml", "--dry-run=client", "--wait", "--timeout", "5m0s"},
		},
		{
			name: "skip wait",
			config: &HelmChartInstallPluginConfig{
				ReleaseName: "my-release",
				Chart:       "my-chart",
			},
			runConfig:      core.RunConfig{SkipWait: ptr.To(true)},
			wantArgsAbsent: []string{"--wait", "--timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			args := buildHelmArgs(tt.config, "/tmp/kubeconfig", tt.valuesPath, tt.runConfig)

			g.Expect(args).To(ContainElements("--kubeconfig", "/tmp/kubeconfig"))
			for _, want := range tt.wantArgsSubset {
				g.Expect(args).To(ContainElement(want))
			}
			for _, absent := range tt.wantArgsAbsent {
				g.Expect(args).NotTo(ContainElement(absent))
			}
		})
	}
}

func Test_HelmChartInstallPlugin_Exec(t *testing.T) {
	tests := []struct {
		name        string
		upgrade     bool
		statusError string
		exitCode    string
		wantSkipped bool
		wantErr     string
	}{
		{name: "skips an existing release by default", exitCode: "0", wantSkipped: true},
		{name: "installs a missing release by default", statusError: "release: not found", exitCode: "0"},
		{name: "upgrades an existing release when enabled", upgrade: true, exitCode: "0"},
		{name: "installs a missing release with upgrade enabled", upgrade: true, statusError: "release: not found", exitCode: "0"},
		{name: "reports upgrade failures", upgrade: true, exitCode: "1", wantErr: "failed to install or upgrade Helm chart my-chart as release my-release"},
		{name: "reports status failures", statusError: "connection refused", wantSkipped: true, wantErr: "failed to check if release my-release already exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			tempDir := t.TempDir()
			helmPath := filepath.Join(tempDir, "helm")
			argsPath := filepath.Join(tempDir, "args")
			valuesPath := filepath.Join(tempDir, "values")
			// A successful status command simulates an existing release.
			g.Expect(os.WriteFile(helmPath, []byte(`#!/bin/sh
case "$1" in
  status)
    if [ -n "$TESTCTL_TEST_HELM_STATUS_ERROR" ]; then
      echo "$TESTCTL_TEST_HELM_STATUS_ERROR" >&2
      exit 1
    fi
    exit 0
    ;;
  upgrade)
    printf '%s\n' "$@" > "$TESTCTL_TEST_HELM_ARGS"
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "-f" ]; then
        cat "$2" > "$TESTCTL_TEST_HELM_VALUES"
        break
      fi
      shift
    done
    if [ "$TESTCTL_TEST_HELM_EXIT_CODE" != "0" ]; then
      echo "upgrade failed" >&2
    fi
    exit "$TESTCTL_TEST_HELM_EXIT_CODE"
    ;;
  *) exit 2 ;;
esac
`), 0o700)).To(Succeed())
			t.Setenv("TESTCTL_HELM_BIN", helmPath)
			t.Setenv("TESTCTL_TEST_HELM_ARGS", argsPath)
			t.Setenv("TESTCTL_TEST_HELM_VALUES", valuesPath)
			t.Setenv("TESTCTL_TEST_HELM_EXIT_CODE", tt.exitCode)
			t.Setenv("TESTCTL_TEST_HELM_STATUS_ERROR", tt.statusError)

			objects := core.TestObjects{core.ManagementKubeConfigKey: []byte("management-kubeconfig")}
			config := &HelmChartInstallPluginConfig{
				ReleaseName: "my-release",
				Chart:       "my-chart",
				Upgrade:     tt.upgrade,
				Version:     "2.0.0",
				Values:      &apiextensionsv1.JSON{Raw: []byte(`{"replicas":3}`)},
			}
			p := HelmChartInstallPlugin{}
			err := p.Exec(t.Context(), fake.NewClientBuilder().Build(), objects, config, core.RunConfig{})
			if tt.wantErr != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			args, err := os.ReadFile(argsPath)
			if tt.wantSkipped {
				g.Expect(os.IsNotExist(err)).To(BeTrue())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(args)).To(HavePrefix("upgrade\n--install\nmy-release\nmy-chart\n"))
			g.Expect(strings.Split(strings.TrimSpace(string(args)), "\n")).To(ContainElements("--version", "2.0.0", "-f"))
			values, err := os.ReadFile(valuesPath)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(values).To(Equal(config.Values.Raw))
		})
	}
}

func Test_HelmChartInstallPlugin_ParseConfig(t *testing.T) {
	g := NewWithT(t)

	p := HelmChartInstallPlugin{}

	t.Run("requires releaseName and chart", func(_ *testing.T) {
		_, err := p.ParseConfig(t.Context(), []byte(`{}`))
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("parses a minimal config", func(_ *testing.T) {
		cfg, err := p.ParseConfig(t.Context(), []byte(`{"releaseName": "r", "chart": "c"}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.(*HelmChartInstallPluginConfig).ReleaseName).To(Equal("r"))
		g.Expect(cfg.(*HelmChartInstallPluginConfig).Upgrade).To(BeFalse())
	})

	t.Run("parses upgrade opt-in", func(t *testing.T) {
		g := NewWithT(t)
		cfg, err := p.ParseConfig(t.Context(), []byte(`{"releaseName": "r", "chart": "c", "upgrade": true}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cfg.(*HelmChartInstallPluginConfig).Upgrade).To(BeTrue())
	})
}
