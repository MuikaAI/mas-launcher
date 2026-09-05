package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MuikaAI/mas-launcher/internal/i18n"
)

const (
	agreementContentFile = "user_agreement.json"
	agreementStateFile   = "user_agreement.json" // inside the instance data dir
	isoDateLayout        = "2006-01-02"
	isoStampLayout       = "2006-01-02T15:04:05" // no tz -> Python fromisoformat-compatible
)

// AgreementContent contains the text and version shown to the user.
type AgreementContent struct {
	Title   string `json:"title"`
	Text    string `json:"text"`
	Updated string `json:"updated"` // ISO date "2006-01-02"
}

// AgreementState mirrors <data>/user_agreement.json. The JSON keys must
// match exactly what muika/utils/first_run.py reads via .get(...).
type AgreementState struct {
	HasAgreed bool   `json:"has_agreed"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// agreementContentPath returns the shared agreement file path in a checkout.
func agreementContentPath(repo string) string {
	bundled := filepath.Join(repo, "muika", agreementContentFile)
	if _, err := os.Stat(bundled); !os.IsNotExist(err) {
		return bundled
	}
	return filepath.Join(repo, "configs", agreementContentFile)
}

// loadAgreementContent reads the shared agreement file. A missing/corrupt
// file or empty fields is an error: silently skipping would leave the bot to
// crash again on its nil-stdin agreement prompt.
func loadAgreementContent(repo string) (AgreementContent, error) {
	var c AgreementContent
	b, err := os.ReadFile(agreementContentPath(repo))
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Title == "" || c.Text == "" || c.Updated == "" {
		return c, errors.New(i18n.T("agreement file has empty title/text/updated"))
	}
	return c, nil
}

// agreementStateDir resolves the instance data dir from DATA_DIR in
// the instance .env (default "data"), joined to the repo when relative.
func agreementStateDir(repo string) string {
	dir := "data"
	if v := parseEnv(filepath.Join(repo, ".env"))["DATA_DIR"]; v != "" {
		dir = v
	}
	if v, ok := os.LookupEnv("DATA_DIR"); ok {
		dir = v
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repo, dir)
	}
	return dir
}

// agreementStatePath returns the signed-state file path.
func agreementStatePath(repo string) string {
	return filepath.Join(agreementStateDir(repo), agreementStateFile)
}

// loadAgreementState reads the signed state. A missing file yields the zero
// state (unsigned); a corrupt file returns an error.
func loadAgreementState(path string) (AgreementState, error) {
	var st AgreementState
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	return st, nil
}

// needsSign reports whether the agreement must be (re-)signed: never agreed,
// or the stored version predates the current one.
func needsSign(hasAgreed bool, storedVersion, updated string) bool {
	if !hasAgreed {
		return true
	}
	if storedVersion == "" || updated == "" {
		return true
	}
	a, err1 := time.Parse(isoDateLayout, storedVersion)
	b, err2 := time.Parse(isoDateLayout, updated)
	if err1 != nil || err2 != nil {
		return true
	}
	return a.Before(b)
}

// sign writes the accepted state into the instance data dir.
func sign(repo string, content AgreementContent) error {
	dir := agreementStateDir(repo)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	st := AgreementState{
		HasAgreed: true,
		Timestamp: time.Now().Format(isoStampLayout),
		Version:   content.Updated,
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, agreementStateFile), append(b, '\n'), 0o600)
}

// confirmAgreement asks the user to accept the license. EOF (non-interactive
// stdin) counts as a decline.
func confirmAgreement(p *prompt) (bool, error) {
	fmt.Print(i18n.T("Agree? (yes/no): "))
	ans, err := p.r.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return false, nil
		}
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "是", "y", "yes":
		return true, nil
	}
	return false, nil
}

// AgreementStatus contains the instance's agreement decision.
type AgreementStatus struct {
	Content         AgreementContent `json:"content"`
	State           AgreementState   `json:"state"`
	StateError      string           `json:"state_error"`
	NeedsAcceptance *bool            `json:"needs_acceptance"`
	shared          bool
}

// agreementCommand keeps protocol output separate from diagnostic messages.
func agreementCommand(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command(python(repo), args...)
	cmd.Dir = repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agreement command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

// readAgreementStatus uses Python when available. Only an absent bridge permits
// legacy handling; a broken bridge must report its error.
func readAgreementStatus(repo string) (AgreementStatus, error) {
	const probe = "import importlib.util, runpy, sys; sys.exit(42) if importlib.util.find_spec('muika.agreement') is None else None; sys.argv = ['muika.agreement', 'status']; runpy.run_module('muika.agreement', run_name='__main__')"
	output, err := agreementCommand(repo, "-c", probe)
	if err == nil {
		var status AgreementStatus
		if err := json.Unmarshal(output, &status); err != nil {
			return status, fmt.Errorf("invalid agreement response: %w", err)
		}
		if status.NeedsAcceptance == nil || status.Content.Title == "" || status.Content.Text == "" || status.Content.Updated == "" {
			return status, errors.New("incomplete agreement response")
		}
		status.shared = true
		return status, nil
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 42 {
		return AgreementStatus{}, err
	}
	content, err := loadAgreementContent(repo)
	if err != nil {
		return AgreementStatus{}, err
	}
	path := agreementStatePath(repo)
	state, err := loadAgreementState(path)
	stateError := ""
	if err != nil {
		stateError = fmt.Sprintf("Cannot read agreement state %s: %v", path, err)
		state = AgreementState{}
	}
	needsAcceptance := needsSign(state.HasAgreed, state.Version, content.Updated)
	return AgreementStatus{Content: content, State: state, StateError: stateError, NeedsAcceptance: &needsAcceptance}, nil
}

// promptAndSign displays the version that the user explicitly accepts.
func (m *Manager) promptAndSign(repo string, status AgreementStatus) error {
	content := status.Content
	fmt.Println(content.Title)
	fmt.Println(content.Text)
	fmt.Printf(i18n.T("The terms were updated on %s. You must agree to the terms and read the license declaration before continuing to use MAS\n"), content.Updated)
	ok, err := confirmAgreement(newPrompt())
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(i18n.T("You did not agree to the agreement; MAS cannot continue running"))
	}
	if status.shared {
		_, err = agreementCommand(repo, "-m", "muika.agreement", "accept", "--version", content.Updated)
	} else {
		err = sign(repo, content)
	}
	if err != nil {
		return err
	}
	fmt.Println(i18n.T("Thank you for agreeing. MAS will now start running"))
	return nil
}

// checkAndSign ensures the license is signed before the instance starts.
func (m *Manager) checkAndSign(repo string) error {
	status, err := readAgreementStatus(repo)
	if err != nil {
		return err
	}
	if status.StateError != "" {
		fmt.Fprintln(os.Stderr, status.StateError)
	}
	if !*status.NeedsAcceptance {
		return nil
	}
	return m.promptAndSign(repo, status)
}

// licenseCmd implements `mas-launcher license [name] [--status]`.
func (m *Manager) licenseCmd(args []string) error {
	name, rest := instanceName(args)
	fs := flag.NewFlagSet("license", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	status := fs.Bool("status", false, "print agreement status only")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	_, repo, err := m.instance(name)
	if err != nil {
		return err
	}
	result, err := readAgreementStatus(repo)
	if err != nil {
		return err
	}
	if result.StateError != "" {
		fmt.Fprintln(os.Stderr, result.StateError)
	}
	if !*result.NeedsAcceptance {
		if *status {
			fmt.Printf(i18n.T("License accepted (version %s, signed %s).\n"), result.State.Version, result.State.Timestamp)
		} else {
			fmt.Printf(i18n.T("License already accepted (version %s).\n"), result.State.Version)
		}
		return nil
	}
	if *status {
		fmt.Printf(i18n.T("License NOT accepted (need version %s).\n"), result.Content.Updated)
		return nil
	}
	return m.promptAndSign(repo, result)
}
