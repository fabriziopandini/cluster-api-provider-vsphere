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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api-provider-vsphere/hack/tools/testctl/core"
)

func Test_resolveKubeconfig(t *testing.T) {
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "default",
		},
	}
	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name(cluster.Name, secret.Kubeconfig),
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: []byte("workload-kubeconfig"),
		},
	}

	t.Run("returns the workload cluster kubeconfig when a Cluster is in TestObjects", func(t *testing.T) {
		g := NewWithT(t)

		c := fake.NewClientBuilder().WithObjects(kubeconfigSecret).Build()
		kubeconfig, target, err := resolveKubeconfig(context.Background(), c, core.TestObjects{ClusterObjectKey: cluster})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(target).To(Equal("workload"))
		g.Expect(kubeconfig).To(Equal([]byte("workload-kubeconfig")))
	})

	t.Run("returns the management cluster kubeconfig when no Cluster is in TestObjects", func(t *testing.T) {
		g := NewWithT(t)

		objects := core.TestObjects{core.ManagementKubeConfigKey: []byte("management-kubeconfig")}

		c := fake.NewClientBuilder().Build()
		kubeconfig, target, err := resolveKubeconfig(context.Background(), c, objects)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(target).To(Equal("management"))
		g.Expect(kubeconfig).To(Equal([]byte("management-kubeconfig")))
	})

	t.Run("errors when the management cluster kubeconfig has not been set", func(t *testing.T) {
		g := NewWithT(t)

		c := fake.NewClientBuilder().Build()
		_, _, err := resolveKubeconfig(context.Background(), c, core.TestObjects{})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("errors when the workload cluster kubeconfig secret does not exist", func(t *testing.T) {
		g := NewWithT(t)

		c := fake.NewClientBuilder().Build()
		_, _, err := resolveKubeconfig(context.Background(), c, core.TestObjects{ClusterObjectKey: cluster})
		g.Expect(err).To(HaveOccurred())
	})
}

func Test_writeTempKubeconfig(t *testing.T) {
	g := NewWithT(t)

	path, cleanup, err := writeTempKubeconfig([]byte("some-kubeconfig"))
	g.Expect(err).NotTo(HaveOccurred())
	defer cleanup()

	data, err := os.ReadFile(path) //nolint:gosec
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(data).To(Equal([]byte("some-kubeconfig")))

	info, err := os.Stat(path)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))

	cleanup()
	_, err = os.Stat(path)
	g.Expect(os.IsNotExist(err)).To(BeTrue())
}

func Test_lookupBinary(t *testing.T) {
	t.Run("returns the env var override when set", func(t *testing.T) {
		g := NewWithT(t)

		t.Setenv("TESTCTL_TEST_BIN", "/some/path/to/bin")
		path, err := lookupBinary("TESTCTL_TEST_BIN", "does-not-matter")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(path).To(Equal("/some/path/to/bin"))
	})

	t.Run("resolves the default name from PATH", func(t *testing.T) {
		g := NewWithT(t)

		path, err := lookupBinary("TESTCTL_TEST_BIN_UNSET", "go")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(path).NotTo(BeEmpty())
	})

	t.Run("errors when the binary cannot be found", func(t *testing.T) {
		g := NewWithT(t)

		_, err := lookupBinary("TESTCTL_TEST_BIN_UNSET", "testctl-totally-nonexistent-binary")
		g.Expect(err).To(HaveOccurred())
	})
}

func Test_countManifestDocuments(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     int
	}{
		{name: "empty manifest", manifest: "", want: 0},
		{name: "blank manifest", manifest: "   \n  ", want: 0},
		{name: "single document", manifest: "kind: ConfigMap\n", want: 1},
		{name: "multiple documents", manifest: "kind: ConfigMap\n---\nkind: Secret\n", want: 2},
		{name: "leading and trailing separators", manifest: "---\nkind: ConfigMap\n---\n", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(countManifestDocuments([]byte(tt.manifest))).To(Equal(tt.want))
		})
	}
}
