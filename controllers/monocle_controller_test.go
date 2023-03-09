package controllers

import (
	"context"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	utils ".test/utils"

	monoclev1alpha1 "github.com/change-metrics/monocle-operator/api/v1alpha1"

	//"github.com/change-metrics/monocle-operator/test/utils"
	corev1 "k8s.io/api/core/v1"
)


var _ = Describe("MonocleController", func() {

	monocleResourceName := "monocle-sample"
	monocleNameSpaceName := "monocle-sample-ns"

	Context("Testing CRD Deployment", func() {

		var monocle *monoclev1alpha1.Monocle
		var monocle_ns *corev1.Namespace
		// var monocle_sa *corev1.ServiceAccount

		ctx := context.Background()
		BeforeEach(func() {
			By("Creating Namespace")
			Expect(utils.CreateNameSapce(monocleNameSpaceName)).To(Succeed())
			By("Creating Monocle CRD")
			Expect(utils.InstallMonocleOperator()).To(Succeed())

			// monocle_sa = &corev1.ServiceAccount{
			// 	ObjectMeta: metav1.ObjectMeta{
			// 		Name:      "monocle-operator-controller-manager",
			// 		Namespace: monocleNameSpaceName,
			// 		Labels: map[string]string{
			// 			"app.kubernetes.io/component":  "rbac",
			// 			"app.kubernetes.io/created-by": "monocle-operator",
			// 			"app.kubernetes.io/instance":   "controller-manager",
			// 			"app.kubernetes.io/managed-by": "kustomize",
			// 			"app.kubernetes.io/name":       "serviceaccount",
			// 			"app.kubernetes.io/part-of":    "monocle-operator",
			// 		},
			// 	},
			// }
		})

		It("Should Create a Monocle Namespace", func() {
			By("Calling the K8s Client")
			Expect(k8sClient.Create(ctx, monocle_ns)).Should(Succeed())
		})

		// It("Should Create a Service Account", func() {
		// 	By("Calling the K8s Client")
		// 	Expect(k8sClient.Create(ctx, monocle_sa)).Should(Succeed())
		// })

		It("Should Create a Monocle Deployment", func() {
			By("Calling the K8s Client")
			cmd := exec.Command("kubectl", "apply", "-f", "https://raw.githubusercontent.com/change-metrics/monocle-operator/master/config/crd/bases/monocle.monocle.change-metrics.io_monocles.yaml")
			//cmd.Run()
			Run(cmd)
			cmd = exec.Command("kubectl", "apply", "-f", "https://raw.githubusercontent.com/change-metrics/monocle-operator/master/install/operator.yml")
			//cmd.Run()
			Run(cmd)
			Expect(k8sClient.Create(ctx, monocle)).Should(Succeed())
		})

		// It("Should Remove a Monocle Deployment", func() {
		// 	By("Calling the K8s Client")
		// 	Expect(k8sClient.Delete(ctx, monocle)).Should(Succeed())
		// })

		// It("Should Remove a Service Account", func() {
		// 	By("Calling the K8s Client")
		// 	Expect(k8sClient.Delete(ctx, monocle_sa)).Should(Succeed())
		// })

		// It("Should Remove a Monocle Namespace", func() {
		// 	By("Calling the K8s Client")
		// 	Expect(k8sClient.Delete(ctx, monocle_ns)).Should(Succeed())
		// })

	})

})
