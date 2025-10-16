package controllers

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8skiwicomv1 "github.com/kiwicom/k8s-vault-operator/api/v1"
)

var _ = Describe("Vault UI URL Annotations", func() {
	Context("KV2 single path", func() {
		It("should add correct annotation for KV2 path", func() {
			vs := &k8skiwicomv1.VaultSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kv2-single",
					Namespace: namespace,
				},
				Spec: k8skiwicomv1.VaultSecretSpec{
					Addr:             "http://127.0.0.1:8200",
					TargetFormat:     "env",
					TargetSecretName: "test-kv2-single-secret",
					Auth: k8skiwicomv1.VaultSecretAuthSpec{
						Token: "testtoken",
					},
					Paths: []k8skiwicomv1.VaultSecretPath{
						{Path: "secret/seeds/team1/project1/secret"},
					},
				},
			}

			err := k8sClient.Create(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			var secret corev1.Secret
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				return err == nil
			}).Should(BeTrue())

			// Check annotation exists
			Expect(secret.Annotations).To(HaveKey("k8s-vault-operator/vault-ui-urls"))

			// Check KV2 URL format (should have /kv/ and URL-encoded path)
			url := secret.Annotations["k8s-vault-operator/vault-ui-urls"]
			Expect(url).To(ContainSubstring("/vault/secrets/secret/kv/"))
			Expect(url).To(ContainSubstring("seeds%2Fteam1%2Fproject1%2Fsecret"))
		})
	})

	Context("KV1 single path", func() {
		It("should add correct annotation for KV1 path", func() {
			vs := &k8skiwicomv1.VaultSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kv1-single",
					Namespace: namespace,
				},
				Spec: k8skiwicomv1.VaultSecretSpec{
					Addr:             "http://127.0.0.1:8200",
					TargetFormat:     "env",
					TargetSecretName: "test-kv1-single-secret",
					Auth: k8skiwicomv1.VaultSecretAuthSpec{
						Token: "testtoken",
					},
					Paths: []k8skiwicomv1.VaultSecretPath{
						{Path: "v1/something/a/secret"},
					},
				},
			}

			err := k8sClient.Create(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			var secret corev1.Secret
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				return err == nil
			}).Should(BeTrue())

			// Check annotation exists
			Expect(secret.Annotations).To(HaveKey("k8s-vault-operator/vault-ui-urls"))

			// Check KV1 URL format (should have /show/)
			url := secret.Annotations["k8s-vault-operator/vault-ui-urls"]
			Expect(url).To(ContainSubstring("/vault/secrets/v1/show/"))
			Expect(url).To(ContainSubstring("something/a/secret"))
		})
	})

	Context("Multiple paths with mixed versions", func() {
		It("should add correct annotations for multiple paths", func() {
			vs := &k8skiwicomv1.VaultSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mixed-paths",
					Namespace: namespace,
				},
				Spec: k8skiwicomv1.VaultSecretSpec{
					Addr:             "http://127.0.0.1:8200",
					TargetFormat:     "env",
					TargetSecretName: "test-mixed-paths-secret",
					Auth: k8skiwicomv1.VaultSecretAuthSpec{
						Token: "testtoken",
					},
					Paths: []k8skiwicomv1.VaultSecretPath{
						{Path: "secret/seeds/team2/project1/secret", Prefix: "kv2_"},
						{Path: "v1/something/a/secret", Prefix: "kv1_"},
					},
				},
			}

			err := k8sClient.Create(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			var secret corev1.Secret
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				return err == nil
			}).Should(BeTrue())

			// Check annotation exists
			Expect(secret.Annotations).To(HaveKey("k8s-vault-operator/vault-ui-urls"))

			// Should contain comma-separated URLs
			urls := secret.Annotations["k8s-vault-operator/vault-ui-urls"]
			Expect(strings.Contains(urls, ",")).To(BeTrue())

			// Check both URL formats are present
			Expect(urls).To(ContainSubstring("/vault/secrets/secret/kv/")) // KV2
			Expect(urls).To(ContainSubstring("/vault/secrets/v1/show/"))   // KV1
		})
	})

	Context("Wildcard paths", func() {
		It("should generate list URLs for wildcard paths", func() {
			vs := &k8skiwicomv1.VaultSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wildcard",
					Namespace: namespace,
				},
				Spec: k8skiwicomv1.VaultSecretSpec{
					Addr:             "http://127.0.0.1:8200",
					TargetFormat:     "env",
					TargetSecretName: "test-wildcard-secret",
					Auth: k8skiwicomv1.VaultSecretAuthSpec{
						Token: "testtoken",
					},
					Paths: []k8skiwicomv1.VaultSecretPath{
						{Path: "secret/seeds/team1/*"},
					},
				},
			}

			err := k8sClient.Create(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			var secret corev1.Secret
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				return err == nil
			}).Should(BeTrue())

			// Check annotation exists
			Expect(secret.Annotations).To(HaveKey("k8s-vault-operator/vault-ui-urls"))

			// Check list URL format for wildcards
			url := secret.Annotations["k8s-vault-operator/vault-ui-urls"]
			Expect(url).To(ContainSubstring("/vault/secrets/secret/kv/list/"))
			Expect(url).To(ContainSubstring("team1/"))
		})
	})

	Context("Annotation preservation on updates", func() {
		It("should preserve existing annotations not managed by operator", func() {
			// First create a VaultSecret
			vs := &k8skiwicomv1.VaultSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-preserve-annot",
					Namespace: namespace,
				},
				Spec: k8skiwicomv1.VaultSecretSpec{
					Addr:             "http://127.0.0.1:8200",
					TargetFormat:     "env",
					TargetSecretName: "test-preserve-annot-secret",
					Auth: k8skiwicomv1.VaultSecretAuthSpec{
						Token: "testtoken",
					},
					Paths: []k8skiwicomv1.VaultSecretPath{
						{Path: "secret/seeds/team1/project1/secret"},
					},
				},
			}

			err := k8sClient.Create(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			var secret corev1.Secret
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				return err == nil
			}).Should(BeTrue())

			// Add a custom annotation directly to the K8s secret
			secret.Annotations["custom-annotation"] = "custom-value"
			err = k8sClient.Update(ctx, &secret)
			Expect(err).ToNot(HaveOccurred())

			// Trigger reconciliation by updating the VaultSecret
			// Need to get latest version to avoid conflict
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: vs.Namespace, Name: vs.Name}, vs)
			Expect(err).ToNot(HaveOccurred())

			vs.Spec.Paths = append(vs.Spec.Paths, k8skiwicomv1.VaultSecretPath{
				Path:   "secret/seeds/team1/project2/secret",
				Prefix: "proj2_",
			})
			err = k8sClient.Update(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			// Wait for update to propagate
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				if err != nil {
					return false
				}
				// Check that URL annotation now has 2 URLs
				urls := secret.Annotations["k8s-vault-operator/vault-ui-urls"]
				return strings.Count(urls, ",") == 1
			}).Should(BeTrue())

			// Verify custom annotation is preserved
			Expect(secret.Annotations).To(HaveKey("custom-annotation"))
			Expect(secret.Annotations["custom-annotation"]).To(Equal("custom-value"))
		})
	})

	Context("Derived UI address from Vault address", func() {
		It("should derive UI address from Vault Addr when not in config", func() {
			// Using actual vault addr but testing that UI URL is derived with /ui suffix
			vs := &k8skiwicomv1.VaultSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-derived-ui-addr",
					Namespace: namespace,
				},
				Spec: k8skiwicomv1.VaultSecretSpec{
					Addr:             "http://127.0.0.1:8200",
					TargetFormat:     "env",
					TargetSecretName: "test-derived-ui-addr-secret",
					Auth: k8skiwicomv1.VaultSecretAuthSpec{
						Token: "testtoken",
					},
					Paths: []k8skiwicomv1.VaultSecretPath{
						{Path: "secret/seeds/team1/project1/secret"},
					},
				},
			}

			err := k8sClient.Create(ctx, vs)
			Expect(err).ToNot(HaveOccurred())

			var secret corev1.Secret
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: vs.Namespace,
					Name:      vs.Spec.TargetSecretName,
				}, &secret)
				return err == nil
			}).Should(BeTrue())

			// Check annotation exists and contains derived UI path
			Expect(secret.Annotations).To(HaveKey("k8s-vault-operator/vault-ui-urls"))
			url := secret.Annotations["k8s-vault-operator/vault-ui-urls"]
			// Should have /ui in the URL (derived from Addr)
			Expect(url).To(ContainSubstring("http://127.0.0.1:8200/ui"))
		})
	})
})
