package controller

import (
	"context"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	core_v1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("SQLInstance Controller", func() {
	ctx := context.Background()

	Context("When reconciling a resource", func() {
		var clientBuilder *fake.ClientBuilder
		var k8sClient client.Client
		var controller *SQLInstanceReconciler

		BeforeEach(func() {
			utilruntime.Must(v1beta1.AddToScheme(scheme.Scheme))
			clientBuilder = fake.NewClientBuilder().
				WithScheme(scheme.Scheme)
		})

		When("the resource exists", func() {
			BeforeEach(func() {
				existingSQLInstance := &v1beta1.SQLInstance{
					TypeMeta: meta_v1.TypeMeta{
						APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
						Kind:       "SQLInstance",
					},
					ObjectMeta: meta_v1.ObjectMeta{
						Name:      "test-instance",
						Namespace: "default",
					},
					Spec: v1beta1.SQLInstanceSpec{},
				}
				clientBuilder = clientBuilder.WithObjects(existingSQLInstance)
			})

			When("no netpol exists", func() {
				BeforeEach(func() {
					k8sClient = clientBuilder.Build()
					controller = &SQLInstanceReconciler{Scheme: scheme.Scheme, Client: k8sClient}
				})

				It("should successfully reconcile the resource", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-instance", Namespace: "default"}}
					result, err := controller.Reconcile(ctx, req)

					Expect(err).ToNot(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))
				})

				It("should create a network policy allowing egress to the ip of the instance", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-instance", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())

					netpol := &v1.NetworkPolicy{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-instance-netpol", Namespace: "default"}, netpol)
					Expect(err).ToNot(HaveOccurred())

					Expect(netpol.Spec.Egress).To(HaveExactElements([]v1.NetworkPolicyEgressRule{
						{Ports: []v1.NetworkPolicyPort{
							{
								Port: &intstr.IntOrString{IntVal: 5432},
							},
						},
							To: []v1.NetworkPolicyPeer{
								{
									IPBlock: &v1.IPBlock{
										CIDR: "",
									},
								},
							},
						},
					}))
				})

				It("should set owner reference and managed by", func() {
					req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-instance", Namespace: "default"}}
					_, err := controller.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())

					netpol := &v1.NetworkPolicy{}
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "sqeletor-instance-netpol", Namespace: "default"}, netpol)
					Expect(err).ToNot(HaveOccurred())

					Expect(netpol.OwnerReferences).To(HaveLen(1))
					Expect(netpol.OwnerReferences[0].Name).To(Equal("test-instance"))
					Expect(netpol.OwnerReferences[0].Kind).To(Equal("SQLInstance"))
					Expect(netpol.OwnerReferences[0].APIVersion).To(Equal("sql.cnrm.cloud.google.com/v1beta1"))

					Expect(netpol.Labels[managedByKey]).To(Equal(sqeletorFqdnId))
				})
			})

			When("a netpol already exists that is not owned or managed", func() {
				BeforeEach(func() {
				})

				It("should not update the network policy", func() {
				})

				It("should not update owner reference or managed by", func() {
				})
			})
		})
	})
})
