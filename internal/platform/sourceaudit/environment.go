package sourceaudit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VerifyBuildEnvironment checks the effective Go environment, including
// settings saved through go env -w, and checks the module cache against sums.
func VerifyBuildEnvironment(root string) error {
	manifest, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	toolchain := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		if name, ok := strings.CutPrefix(line, "toolchain "); ok {
			toolchain = strings.TrimSpace(name)
		}
	}
	command := exec.Command("go", "env", "-json", "GOFLAGS", "GOWORK", "GOEXPERIMENT", "GOVERSION")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("read approved Go output build environment: %w", err)
	}
	var settings map[string]string
	if err := json.Unmarshal(output, &settings); err != nil {
		return err
	}
	if settings["GOFLAGS"] != "" || settings["GOEXPERIMENT"] != "" || settings["GOWORK"] != "" && settings["GOWORK"] != "off" {
		return fmt.Errorf("unsupported Go output build overrides (GOFLAGS, GOEXPERIMENT, or active GOWORK)")
	}
	if len(toolchain) == 0 || settings["GOVERSION"] != toolchain {
		return fmt.Errorf("Go output audit requires the toolchain pinned in go.mod")
	}
	command = exec.Command("go", "mod", "verify")
	command.Dir = root
	if err := command.Run(); err != nil {
		return fmt.Errorf("Go dependency cache failed module verification: %w", err)
	}
	return nil
}
