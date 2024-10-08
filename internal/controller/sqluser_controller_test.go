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

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("SQLUser Controller", func() {
	ctx := context.Background()

	Context("When reconciling a resource", func() {
		var clientBuilder *fake.ClientBuilder
		var k8sClient client.Client
		var controller *SQLUserReconciler

		const (
			instanceIP        = "10.10.10.10"
			dbName            = "test-db"
			userName          = "test-user"
			resourceId        = "test-resource-id"
			envVarPrefix      = "PREFIX"
			secretName        = "test-secret-env"
			secretKey         = "PREFIX_PASSWORD"
			instanceName      = "test-instance"
			namespace         = "default"
			databaseEnvVarKey = envVarPrefix + "_DATABASE"
		)

		BeforeEach(func() {
			utilruntime.Must(v1beta1.AddToScheme(scheme.Scheme))
			clientBuilder = fake.NewClientBuilder().
				WithScheme(scheme.Scheme)
		})

		When("the resource exists", func() {
			BeforeEach(func() {
				existingUser := &v1beta1.SQLUser{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
						Kind:       "SQLUser",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      userName,
						Namespace: namespace,
						Annotations: map[string]string{
							"sqeletor.nais.io/env-var-prefix": envVarPrefix,
							"sqeletor.nais.io/database-name":  dbName,
						},
					},
					Spec: v1beta1.SQLUserSpec{
						Password: &v1beta1.UserPassword{
							ValueFrom: &v1beta1.UserValueFrom{
								SecretKeyRef: &v1alpha1.SecretKeyRef{
									Name: secretName,
									Key:  secretKey,
								},
							},
						},
						InstanceRef: v1alpha1.ResourceRef{
							Name:      instanceName,
							Namespace: namespace,
						},
						ResourceID: ptr.To(resourceId),
					},
				}

				clientBuilder = clientBuilder.WithObjects(existingUser)
			})

			When("sql instance exists and is ready", func() {
				BeforeEach(func() {
					existingSqlInstance := &v1beta1.SQLInstance{
						TypeMeta: meta_v1.TypeMeta{
							APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
							Kind:       "SQLInstance",
						},
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      instanceName,
							Namespace: namespace,
						},
						Spec: v1beta1.SQLInstanceSpec{
							Settings: v1beta1.InstanceSettings{
								IpConfiguration: &v1beta1.InstanceIpConfiguration{
									PrivateNetworkRef: &v1alpha1.ResourceRef{
										Name: "test-network",
									},
								},
							},
						},
						Status: v1beta1.SQLInstanceStatus{
							PrivateIpAddress: ptr.To(instanceIP),
						},
					}

					clientBuilder = clientBuilder.WithObjects(existingSqlInstance)
				})

				When("no secret exists", func() {
					BeforeEach(func() {
						k8sClient = clientBuilder.Build()
						controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}
					})

					It("should successfully reconcile the resource", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						result, err := controller.Reconcile(ctx, req)

						Expect(err).ToNot(HaveOccurred())
						Expect(result).To(Equal(ctrl.Result{}))
					})

					It("should create a secret containing the env vars", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						Expect(secret.StringData).To(HaveKey(envVarPrefix + "_PASSWORD"))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_HOST", instanceIP))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_PORT", "5432"))
						Expect(secret.StringData).To(HaveKeyWithValue(databaseEnvVarKey, dbName))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_USERNAME", resourceId))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_SSLROOTCERT", "/var/run/secrets/nais.io/sqlcertificate/root-cert.pem"))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_SSLCERT", "/var/run/secrets/nais.io/sqlcertificate/cert.pem"))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_SSLKEY", "/var/run/secrets/nais.io/sqlcertificate/key.pem"))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_SSLKEY_PK8", "/var/run/secrets/nais.io/sqlcertificate/key.pk8"))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_SSLMODE", "verify-ca"))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_URL", MatchRegexp(`^postgresql:\/\/test-resource-id:[^@]+@10.10.10.10:5432\/test-db\?sslcert=%2Fvar%2Frun%2Fsecrets%2Fnais.io%2Fsqlcertificate%2Fcert.pem&sslkey=%2Fvar%2Frun%2Fsecrets%2Fnais.io%2Fsqlcertificate%2Fkey.pem&sslmode=verify-ca&sslrootcert=%2Fvar%2Frun%2Fsecrets%2Fnais.io%2Fsqlcertificate%2Froot-cert.pem$`)))
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_JDBC_URL", MatchRegexp(`^jdbc:postgresql:\/\/10.10.10.10:5432\/test-db\?password=[^@]+&sslcert=%2Fvar%2Frun%2Fsecrets%2Fnais.io%2Fsqlcertificate%2Fcert.pem&sslkey=%2Fvar%2Frun%2Fsecrets%2Fnais.io%2Fsqlcertificate%2Fkey.pk8&sslmode=verify-ca&sslrootcert=%2Fvar%2Frun%2Fsecrets%2Fnais.io%2Fsqlcertificate%2Froot-cert.pem&user=test-resource-id$`)))
					})

					It("should set owner reference and managed by", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						Expect(secret.OwnerReferences).To(HaveLen(1))
						Expect(secret.OwnerReferences[0].Name).To(Equal(userName))
						Expect(secret.OwnerReferences[0].Kind).To(Equal("SQLUser"))
						Expect(secret.OwnerReferences[0].APIVersion).To(Equal("sql.cnrm.cloud.google.com/v1beta1"))

						lastUpdated, err := time.Parse(time.RFC3339, secret.Annotations[lastUpdatedAnnotation])
						Expect(err).ToNot(HaveOccurred())
						Expect(lastUpdated).To(BeTemporally("~", time.Now(), 5*time.Second))

						Expect(secret.Labels[managedByKey]).To(Equal(sqeletorFqdnId))
					})
				})

				When("a secret already exists that is not owned or managed", func() {
					BeforeEach(func() {
						existingSecret := &core_v1.Secret{
							ObjectMeta: meta_v1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
								CreationTimestamp: meta_v1.Time{
									Time: time.Now(),
								},
							},
						}
						k8sClient = clientBuilder.WithObjects(existingSecret).Build()
						controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}
					})

					It("should not update the secret with the config data", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						Expect(secret.StringData).To(BeEmpty())
					})

					It("should not update owner reference or managed by", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						Expect(secret.OwnerReferences).To(HaveLen(0))
						Expect(secret.Labels[managedByKey]).To(BeEmpty())
					})
				})

				When("a secret already exists that is owned and managed by correct user", func() {
					BeforeEach(func() {
						existingSecret := &core_v1.Secret{
							ObjectMeta: meta_v1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
								CreationTimestamp: meta_v1.Time{
									Time: time.Now(),
								},
								Labels: map[string]string{
									managedByKey: sqeletorFqdnId,
								},
								OwnerReferences: []meta_v1.OwnerReference{
									{
										APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
										Kind:       "SQLUser",
										Name:       userName,
									},
								},
							},
							Data: map[string][]byte{
								envVarPrefix + "_PASSWORD": []byte("testpassword"),
							},
							StringData: map[string]string{
								databaseEnvVarKey: "something-else",
							},
						}
						k8sClient = clientBuilder.WithObjects(existingSecret).Build()
						controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}
					})

					It("should update the secret with the env data", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).ToNot(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						// just test one value, the rest is tested in a previous test
						Expect(secret.StringData).To(HaveKeyWithValue(databaseEnvVarKey, dbName))
						// password should not be updated
						Expect(secret.StringData).To(HaveKeyWithValue(envVarPrefix+"_PASSWORD", "testpassword"))
					})
				})

				When("a secret already exists that is owned and managed by other user", func() {
					BeforeEach(func() {
						existingSecret := &core_v1.Secret{
							ObjectMeta: meta_v1.ObjectMeta{
								Name:      secretName,
								Namespace: namespace,
								CreationTimestamp: meta_v1.Time{
									Time: time.Now(),
								},
								Labels: map[string]string{
									managedByKey: sqeletorFqdnId,
								},
								OwnerReferences: []meta_v1.OwnerReference{
									{
										APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
										Kind:       "SQLUser",
										Name:       "other-user",
									},
								},
							},
							StringData: map[string]string{
								databaseEnvVarKey: "something-else",
							},
						}
						k8sClient = clientBuilder.WithObjects(existingSecret).Build()
						controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}
					})

					It("should not update the secret with the env data", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						Expect(secret.StringData).To(HaveKeyWithValue(databaseEnvVarKey, "something-else"))
					})

					It("should leave owner reference alone", func() {
						req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
						_, err := controller.Reconcile(ctx, req)
						Expect(err).To(HaveOccurred())

						secret := &core_v1.Secret{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
						Expect(err).ToNot(HaveOccurred())

						Expect(secret.OwnerReferences).To(HaveLen(1))
						Expect(secret.OwnerReferences[0].Name).To(Equal("other-user"))
						Expect(secret.OwnerReferences[0].Kind).To(Equal("SQLUser"))
						Expect(secret.OwnerReferences[0].APIVersion).To(Equal("sql.cnrm.cloud.google.com/v1beta1"))
					})
				})
			})
			When("sql instance exists but is not configured for private ip", func() {
				It("should return a permanent error", func() {
					existingSqlInstance := &v1beta1.SQLInstance{
						TypeMeta: meta_v1.TypeMeta{
							APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
							Kind:       "SQLInstance",
						},
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      instanceName,
							Namespace: namespace,
						},
						Spec: v1beta1.SQLInstanceSpec{
							Settings: v1beta1.InstanceSettings{
								IpConfiguration: &v1beta1.InstanceIpConfiguration{},
							},
						},
					}

					clientBuilder = clientBuilder.WithObjects(existingSqlInstance)
					k8sClient = clientBuilder.Build()
					controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}

					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError("permanent failure: referenced sql instance is not configured for private ip"))
				})
			})

			When("sql instance exists but does not have a private ip yet", func() {
				It("should return a temporary error", func() {
					existingSqlInstance := &v1beta1.SQLInstance{
						TypeMeta: meta_v1.TypeMeta{
							APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
							Kind:       "SQLInstance",
						},
						ObjectMeta: meta_v1.ObjectMeta{
							Name:      instanceName,
							Namespace: namespace,
						},
						Spec: v1beta1.SQLInstanceSpec{
							Settings: v1beta1.InstanceSettings{
								IpConfiguration: &v1beta1.InstanceIpConfiguration{
									PrivateNetworkRef: &v1alpha1.ResourceRef{
										Name: "test-network",
									},
								},
							},
						},
					}

					clientBuilder = clientBuilder.WithObjects(existingSqlInstance)
					k8sClient = clientBuilder.Build()
					controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}

					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
					result, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{RequeueAfter: time.Minute}))
				})
			})
			When("sql instance does not exist", func() {
				It("should return a temporary error", func() {
					k8sClient = clientBuilder.Build()
					controller = &SQLUserReconciler{Scheme: scheme.Scheme, Client: k8sClient}

					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: userName, Namespace: namespace}}
					result, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{RequeueAfter: time.Minute}))
				})
			})
		})
	})
})
