package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core_v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SQLSSLCert Controller", func() {
	ctx := context.Background()

	testKey := `
-----BEGIN RSA PRIVATE KEY-----
MIIBOwIBAAJBAKxZ8OQ2RkTHffug5/194IXuJNw19zI15twhJ0lxSzdzcsz3ApeF
0nA1iGdu2g70W3VnGA+4jm0UprjcJmUCxc8CAwEAAQJBAKczEcizBnRO+98SeDyo
0xnar5OaHtdtBytiVlSfPhLpqvSdN1ydLw7sDvDUu9slE4dDJTMgdHGNgq0FNeRa
EsECIQDbTx6p45xsm5I6iG1xYn+8X3hX+J8Y5VKb6/vgpSddkQIhAMkvphhiI2nj
kmjJ/wvrJSq1fnjgJYAOQMPNUcb4o71fAiBwnv3ZMpimsXFze5HwUyvTmZdcXcGd
8E3u4k2zvDwt8QIhAL1HQQMLwbmry2EfOf8imfMWkghzCZTy0+fjUZ7a6mINAiAj
KChGB9mxeIDV+wqRFCOK0IVOlBk4e+O2mk31LrXibw==
-----END RSA PRIVATE KEY-----`

	testDerKey := []byte{48, 130, 1, 85, 2, 1, 0, 48, 13, 6, 9, 42, 134, 72, 134, 247, 13, 1, 1, 1, 5, 0, 4, 130, 1, 63, 48, 130, 1, 59, 2, 1, 0, 2, 65, 0, 172, 89, 240, 228, 54, 70, 68, 199, 125, 251, 160, 231, 253, 125, 224, 133, 238, 36, 220, 53, 247, 50, 53, 230, 220, 33, 39, 73, 113, 75, 55, 115, 114, 204, 247, 2, 151, 133, 210, 112, 53, 136, 103, 110, 218, 14, 244, 91, 117, 103, 24, 15, 184, 142, 109, 20, 166, 184, 220, 38, 101, 2, 197, 207, 2, 3, 1, 0, 1, 2, 65, 0, 167, 51, 17, 200, 179, 6, 116, 78, 251, 223, 18, 120, 60, 168, 211, 25, 218, 175, 147, 154, 30, 215, 109, 7, 43, 98, 86, 84, 159, 62, 18, 233, 170, 244, 157, 55, 92, 157, 47, 14, 236, 14, 240, 212, 187, 219, 37, 19, 135, 67, 37, 51, 32, 116, 113, 141, 130, 173, 5, 53, 228, 90, 18, 193, 2, 33, 0, 219, 79, 30, 169, 227, 156, 108, 155, 146, 58, 136, 109, 113, 98, 127, 188, 95, 120, 87, 248, 159, 24, 229, 82, 155, 235, 251, 224, 165, 39, 93, 145, 2, 33, 0, 201, 47, 166, 24, 98, 35, 105, 227, 146, 104, 201, 255, 11, 235, 37, 42, 181, 126, 120, 224, 37, 128, 14, 64, 195, 205, 81, 198, 248, 163, 189, 95, 2, 32, 112, 158, 253, 217, 50, 152, 166, 177, 113, 115, 123, 145, 240, 83, 43, 211, 153, 151, 92, 93, 193, 157, 240, 77, 238, 226, 77, 179, 188, 60, 45, 241, 2, 33, 0, 189, 71, 65, 3, 11, 193, 185, 171, 203, 97, 31, 57, 255, 34, 153, 243, 22, 146, 8, 115, 9, 148, 242, 211, 231, 227, 81, 158, 218, 234, 98, 13, 2, 32, 35, 40, 40, 70, 7, 217, 177, 120, 128, 213, 251, 10, 145, 20, 35, 138, 208, 133, 78, 148, 25, 56, 123, 227, 182, 154, 77, 245, 46, 181, 226, 111}

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
						PrivateKey:   ptr.To(testKey),
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
					Expect(secret.StringData).To(HaveKeyWithValue(pemKeyKey, testKey))
					Expect(secret.StringData).To(HaveKeyWithValue(rootCertKey, "dummy-server-ca-cert"))
					Expect(secret.Data).To(HaveKeyWithValue(derKeyKey, testDerKey))
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

					Expect(secret.Labels[managedByKey]).To(Equal(sqeletorFqdnId))
				})
			})

			When("a secret already exists that is not owned or managed", func() {
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

			When("a secret already exists that is owned and managed by correct cert", func() {
				BeforeEach(func() {
					existingSecret := &core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      "sqeletor-test-secret",
							Namespace: "default",
							CreationTimestamp: meta_v1.Time{
								Time: time.Now(),
							},
							Labels: map[string]string{
								managedByKey: sqeletorFqdnId,
							},
							OwnerReferences: []meta_v1.OwnerReference{
								{
									APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
									Kind:       "SQLSSLCert",
									Name:       "test-cert",
								},
							},
						},
					}
					k8sClient = clientBuilder.WithObjects(existingSecret).Build()
					controller = &SQLSSLCertReconciler{Scheme: scheme.Scheme, Client: k8sClient}
				})

				It("should update the secret with the certificate data", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())

					secret := &core_v1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-test-secret", Namespace: "default"}, secret)
					Expect(err).ToNot(HaveOccurred())

					Expect(secret.StringData).To(HaveKeyWithValue(certKey, "dummy-cert"))
					Expect(secret.StringData).To(HaveKeyWithValue(pemKeyKey, testKey))
					Expect(secret.StringData).To(HaveKeyWithValue(rootCertKey, "dummy-server-ca-cert"))
				})
			})

			When("a secret already exists that is owned and managed by other cert", func() {
				BeforeEach(func() {
					existingSecret := &core_v1.Secret{
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      "sqeletor-test-secret",
							Namespace: "default",
							CreationTimestamp: meta_v1.Time{
								Time: time.Now(),
							},
							Labels: map[string]string{
								managedByKey: sqeletorFqdnId,
							},
							OwnerReferences: []meta_v1.OwnerReference{
								{
									APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
									Kind:       "SQLSSLCert",
									Name:       "other-cert",
								},
							},
						},
						StringData: map[string]string{
							certKey:     "existing-cert",
							pemKeyKey:   "existing-private-key",
							rootCertKey: "existing-server-ca-cert",
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

					Expect(secret.StringData).To(HaveKeyWithValue(certKey, "existing-cert"))
					Expect(secret.StringData).To(HaveKeyWithValue(pemKeyKey, "existing-private-key"))
					Expect(secret.StringData).To(HaveKeyWithValue(rootCertKey, "existing-server-ca-cert"))
				})

				It("should leave owner reference alone", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).To(HaveOccurred())

					secret := &core_v1.Secret{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-test-secret", Namespace: "default"}, secret)
					Expect(err).ToNot(HaveOccurred())

					Expect(secret.OwnerReferences).To(HaveLen(1))
					Expect(secret.OwnerReferences[0].Name).To(Equal("other-cert"))
					Expect(secret.OwnerReferences[0].Kind).To(Equal("SQLSSLCert"))
					Expect(secret.OwnerReferences[0].APIVersion).To(Equal("sql.cnrm.cloud.google.com/v1beta1"))
				})
			})
		})
	})
})
