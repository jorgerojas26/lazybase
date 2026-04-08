package supabase

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Status struct {
	State     string
	StudioURL string
	Details   string
}

type ExitError struct {
	code int
	err  error
}

func NewExitError(code int, err error) ExitError {
	return ExitError{code: code, err: err}
}

func (e ExitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("process exited with code %d", e.code)
	}
	return e.err.Error()
}

func (e ExitError) Unwrap() error {
	return e.err
}

func (e ExitError) ExitCode() int {
	return e.code
}

func LookPath() (string, error) {
	path, err := exec.LookPath("supabase")
	if err != nil {
		return "", errors.New("supabase CLI not found in PATH; install it first")
	}
	return path, nil
}

func Run(args []string) error {
	_, err := LookPath()
	if err != nil {
		return err
	}

	cmd := exec.Command("supabase", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExitError{code: exitErr.ExitCode(), err: exitErr}
		}
		return fmt.Errorf("run supabase %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func Start(extraArgs []string) error {
	return StartWithWorkdir("", extraArgs)
}

func StartWithWorkdir(workdir string, extraArgs []string) error {
	args := []string{"start"}
	if workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	args = append(args, extraArgs...)
	return Run(args)
}

func StatusForProject(workdir string) (Status, error) {
	if _, err := LookPath(); err != nil {
		return Status{State: "unknown"}, err
	}

	jsonCmd := exec.Command("supabase", "status", "--workdir", workdir, "-o", "json")
	output, err := jsonCmd.Output()
	if err == nil {
		status := parseJSONStatus(output)
		if status.State == "" {
			status.State = "running"
		}
		return status, nil
	}

	prettyCmd := exec.Command("supabase", "status", "--workdir", workdir)
	prettyOutput, prettyErr := prettyCmd.Output()
	if prettyErr != nil {
		return Status{State: "unknown"}, prettyErr
	}

	status := parsePrettyStatus(prettyOutput)
	if status.State == "" {
		status.State = "running"
	}
	return status, nil
}

func parseJSONStatus(raw []byte) Status {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Status{State: "running"}
	}

	status := Status{State: "running"}
	visitJSON(payload, func(key string, value any) {
		text, ok := value.(string)
		if !ok {
			return
		}

		lowerKey := strings.ToLower(key)
		lowerValue := strings.ToLower(text)
		if strings.Contains(lowerKey, "studio") && strings.HasPrefix(text, "http") && status.StudioURL == "" {
			status.StudioURL = text
		}
		if strings.Contains(lowerKey, "status") || strings.Contains(lowerKey, "state") {
			if strings.Contains(lowerValue, "stopped") {
				status.State = "stopped"
			}
		}
	})

	return status
}

func visitJSON(value any, fn func(key string, value any)) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			fn(key, child)
			visitJSON(child, fn)
		}
	case []any:
		for _, child := range typed {
			visitJSON(child, fn)
		}
	}
}

func parsePrettyStatus(raw []byte) Status {
	status := Status{}
	for _, line := range strings.Split(string(bytes.TrimSpace(raw)), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		label := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		lowerLabel := strings.ToLower(label)
		lowerValue := strings.ToLower(value)

		if strings.Contains(lowerLabel, "studio") && strings.HasPrefix(value, "http") {
			status.StudioURL = value
		}
		if strings.Contains(lowerLabel, "status") || strings.Contains(lowerLabel, "state") {
			status.State = lowerValue
		}
	}
	return status
}
