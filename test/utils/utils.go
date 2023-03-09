package utils

import (
	"fmt"
	"os/exec"
	"strings"
	//. "github.com/onsi/ginkgo/v2" //nolint:golint,revive
)

const monocleUrl = "https://raw.githubusercontent.com/change-metrics/monocle-operator/master/config/crd/bases/monocle.monocle.change-metrics.io_monocles.yaml"

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

func InstallMonocleOperator() ([]byte, error) {
	return ExecuteCmd("kubectl", "apply", "-f", monocleUrl)
}

func UninstallMonocleOperator() ([]byte, error) {
	return ExecuteCmd("kubectl", "delete", "-f", monocleUrl)
}

func CreateNameSpace(namespace string) ([]byte, error) {
	return ExecuteCmd("kubectl", "create", "namespace", namespace)
}

func DeleteNameSpace(namespace string) ([]byte, error) {
	return ExecuteCmd("kubectl", "delete", "namespace", namespace)
}
