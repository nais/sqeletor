package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("SQLSSLCert Controller", func() {
	ctx := context.Background()
	Context("When reconciling a resource", func() {

		It("should successfully reconcile the resource", func() {
			existingCert := &v1beta1.SQLSSLCert{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-cert",
					Namespace: "default",
					Annotations: map[string]string{
						"sqeletor.nais.io/secret-name": "sqeletor-test-secret",
					},
				},
			}

			Expect(k8sClient.Create(ctx, existingCert)).To(Succeed())

			controller := &SQLSSLCertReconciler{Scheme: scheme.Scheme, Client: k8sClient}

			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test-cert", Namespace: "default"}}
			result, err := controller.Reconcile(ctx, req)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
