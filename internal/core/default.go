package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MuikaAI/mas-launcher/internal/i18n"
	"github.com/MuikaAI/mas-launcher/internal/models"
)

// envCheckIDs is the stable order in which environment completeness is
// checked by envStatus and repaired by defaultCmd.
var envCheckIDs = []string{"python", "env-file", "master-id", "model"}

// envStatus reports the environment completeness problems for the instance
// at repo, in envCheckIDs order. An empty result means the environment is
// ready to start. A corrupt configs/models.yml propagates its error.
func (m *Manager) envStatus(repo string) ([]string, error) {
	var missing []string
	if _, err := os.Stat(python(repo)); err != nil {
		missing = append(missing, "python")
	}
	envPath := filepath.Join(repo, ".env")
	if _, err := os.Stat(envPath); err != nil {
		missing = append(missing, "env-file")
	} else if parseEnv(envPath)["MASTER_ID"] == "" {
		// Only checked when .env exists: an absent file is already covered
		// by "env-file", so each id maps to exactly one fix.
		missing = append(missing, "master-id")
	}
	mf, err := models.LoadModelFile(models.ModelsPath(repo))
	if err != nil {
		return nil, err
	}
	if !models.HasAnyDefault(mf) {
		missing = append(missing, "model")
	}
	return missing, nil
}

// defaultCmd implements the no-argument bootstrap: it brings the default
// instance to a runnable state (creating it when absent, repairing or
// guiding through missing configuration) and then starts Core and Bot.
func (m *Manager) defaultCmd(args []string) error {
	const name = "default"
	if _, ok := m.Config.Instances[name]; !ok {
		fmt.Println(i18n.T("No default instance found; creating one..."))
		if err := m.initCmd(nil); err != nil {
			return err
		}
	}
	i, repo, err := m.instance(name)
	if err != nil {
		return err
	}
	if s, e := m.readState(name); e == nil && (processAlive(s.CorePID) || processAlive(s.BotPID)) {
		fmt.Println(i18n.T("Instance already running."))
		return nil
	}
	missing, err := m.envStatus(repo)
	if err != nil {
		return err
	}
	need := make(map[string]bool, len(missing))
	for _, id := range missing {
		need[id] = true
	}
	for _, id := range envCheckIDs {
		if !need[id] {
			fmt.Printf(i18n.T("  ok: %s\n"), i18n.T(id))
			continue
		}
		fmt.Printf(i18n.T("  %s missing; fixing...\n"), i18n.T(id))
		var e error
		switch id {
		case "python":
			e = m.ensureEnvironment(repo)
		case "env-file":
			e = m.defaultFiles(repo, i)
		case "master-id":
			e = m.configureCmd(nil)
		case "model":
			e = m.modelCmd(nil)
		}
		if e != nil {
			return e
		}
	}
	if rest, e := m.envStatus(repo); e == nil && len(rest) > 0 {
		fmt.Printf(i18n.T("Warning: still missing: %v\n"), rest)
	}
	return m.startCmd(nil)
}
