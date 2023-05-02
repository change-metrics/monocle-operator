package controllers

import (
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const monocleCRDUrl = "https://raw.githubusercontent.com/change-metrics/monocle-operator/master/config/crd/bases/monocle.monocle.change-metrics.io_monocles.yaml"

const monocleOperatorUrl = "https://raw.githubusercontent.com/change-metrics/monocle-operator/master/install/operator.yml"

const monocleManifestUrl = "https://raw.githubusercontent.com/change-metrics/monocle-operator/master/config/samples/monocle_v1alpha1_monocle-alt.yaml"

func ExecuteCmd(command ...string) ([]byte, error) {
	// Command to be executed
	args := command[1:]
	cmd := exec.Command(command[0], args...)

	// Executing Command
	stdoutanderr, err := cmd.CombinedOutput()
	if err != nil {
		return stdoutanderr, fmt.Errorf("%s failed with error: (%v) %s", strings.Join(cmd.Args, " "), err, string(stdoutanderr))
	}
	return stdoutanderr, err
}

func CreateMonocleOperator() ([]byte, error) {
	return ExecuteCmd("kubectl", "create", "-f", monocleOperatorUrl)
}

func DeleteMonocleOperator() ([]byte, error) {
	return ExecuteCmd("kubectl", "delete", "-f", monocleOperatorUrl)
}

func CreateMonocleCRD() ([]byte, error) {
	return ExecuteCmd("kubectl", "create", "-f", monocleCRDUrl)
}

func DeleteMonocleCRD() ([]byte, error) {
	return ExecuteCmd("kubectl", "delete", "-f", monocleCRDUrl)
}

func CreateMonocleInstance() ([]byte, error) {
	return ExecuteCmd("kubectl", "create", "-f", monocleManifestUrl)
}

func DeleteMonocleInstance() ([]byte, error) {
	return ExecuteCmd("kubectl", "delete", "-f", monocleManifestUrl)
}

func CreateNameSpace(namespace string) ([]byte, error) {
	return ExecuteCmd("kubectl", "create", "namespace", namespace)
}

func DeleteNameSpace(namespace string) ([]byte, error) {
	return ExecuteCmd("kubectl", "delete", "namespace", namespace)
}

var _ = Describe("MonocleController", func() {

	Context("Testing CRD Deployment", func() {

		BeforeEach(func() {

		})

		AfterEach(func() {

		})

		It("Should Create Monocle Instance", func() {
			By("Calling the K8s Client")
			_, err := CreateMonocleInstance()
			Expect(err).To(Succeed())
		})

		It("Should Delete Monocle Instance", func() {
			By("Calling the K8s Client")
			_, err := DeleteMonocleInstance()
			Expect(err).To(Succeed())
		})

	})

})
