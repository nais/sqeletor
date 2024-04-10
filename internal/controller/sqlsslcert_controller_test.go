package controller

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core_v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"time"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SQLSSLCert Controller", func() {
	ctx := context.Background()

	Context("When reconciling a resource", func() {
		var clientBuilder *fake.ClientBuilder
		var k8sClient client.Client
		var controller *SQLSSLCertReconciler

		BeforeEach(func() {
			utilruntime.Must(v1beta1.AddToScheme(scheme.Scheme))
			clientBuilder = fake.NewClientBuilder().
				WithScheme(scheme.Scheme)
		})

		When("the resource exists", func() {
			BeforeEach(func() {
				existingCert := &v1beta1.SQLSSLCert{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
						Kind:       "SQLSSLCert",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "test-cert",
						Namespace: "default",
						Annotations: map[string]string{
							"sqeletor.nais.io/secret-name": "sqeletor-test-secret",
						},
					},
					Spec: v1beta1.SQLSSLCertSpec{},
					Status: v1beta1.SQLSSLCertStatus{
						Cert:         ptr.To("dummy-cert"),
						PrivateKey:   ptr.To("dummy-private-key"),
						ServerCaCert: ptr.To("dummy-server-ca-cert"),
					},
				}

				clientBuilder = clientBuilder.WithObjects(existingCert)
			})

			When("no secret exists", func() {
				BeforeEach(func() {
					k8sClient = clientBuilder.Build()
					controller = &SQLSSLCertReconciler{Scheme: scheme.Scheme, Client: k8sClient}
				})

				It("should successfully reconcile the resource", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					result, err := controller.Reconcile(ctx, req)

					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))
				})

				It("should create a secret containing the certificate data", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())

					secret := &core_v1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-test-secret", Namespace: "default"}, secret)
					Expect(err).ToNot(HaveOccurred())

					Expect(secret.StringData).To(HaveKeyWithValue(certKey, "dummy-cert"))
					Expect(secret.StringData).To(HaveKeyWithValue(privateKeyKey, "dummy-private-key"))
					Expect(secret.StringData).To(HaveKeyWithValue(serverCaCertKey, "dummy-server-ca-cert"))
				})

				It("should set owner reference and managed by", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())

					secret := &core_v1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-test-secret", Namespace: "default"}, secret)
					Expect(err).ToNot(HaveOccurred())

					Expect(secret.OwnerReferences).To(HaveLen(1))
					Expect(secret.OwnerReferences[0].Name).To(Equal("test-cert"))
					Expect(secret.OwnerReferences[0].Kind).To(Equal("SQLSSLCert"))
					Expect(secret.OwnerReferences[0].APIVersion).To(Equal("sql.cnrm.cloud.google.com/v1beta1"))

					Expect(secret.Labels[managedByKey]).To(Equal("sqeletor.nais.io"))
				})
			})

			When("a secret already exists", func() {
				BeforeEach(func() {
					existingSecret := &core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      "sqeletor-test-secret",
							Namespace: "default",
							CreationTimestamp: meta_v1.Time{
								Time: time.Now(),
							},
						},
					}
					k8sClient = clientBuilder.WithObjects(existingSecret).Build()
					controller = &SQLSSLCertReconciler{Scheme: scheme.Scheme, Client: k8sClient}
				})

				It("should not update the secret with the certificate data", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).To(HaveOccurred())

					secret := &core_v1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-test-secret", Namespace: "default"}, secret)
					Expect(err).ToNot(HaveOccurred())

					Expect(secret.StringData).To(BeEmpty())
				})

				It("should not update owner reference or managed by", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).To(HaveOccurred())

					secret := &core_v1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-test-secret", Namespace: "default"}, secret)
					Expect(err).ToNot(HaveOccurred())

					Expect(secret.OwnerReferences).To(HaveLen(0))
					Expect(secret.Labels[managedByKey]).To(BeEmpty())
				})
			})
		})
	})
})
