package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// SweepScriptRun is the observable result of running a rendered session-sweep command.
type SweepScriptRun struct {
	ExitCode int
	Stdout   string
	Stderr   string
	// CurlArgv and CurlEnv are what the curl process received, recorded by a PATH shim.
	CurlArgv string
	CurlEnv  string
}

// RunSweepScript runs the first container command of a rendered session-sweep CronJob with
// the local /bin/sh. A recording shim placed first on PATH captures curl's argv and
// environment and then executes the real curl, so tests can assert which values reach the
// curl process. The shim only records; all assertions stay in Go.
func RunSweepScript(cronJob map[string]any, timeout time.Duration) (SweepScriptRun, error) {
	command, err := sweepCommand(cronJob)
	if err != nil {
		return SweepScriptRun{}, err
	}
	realCurl, err := exec.LookPath("curl")
	if err != nil {
		return SweepScriptRun{}, fmt.Errorf("curl not found on PATH: %w", err)
	}
	workDir, err := os.MkdirTemp("", "sweep-script-*")
	if err != nil {
		return SweepScriptRun{}, err
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	shimDir := filepath.Join(workDir, "bin")
	recordDir := filepath.Join(workDir, "record")
	for _, dir := range []string{shimDir, recordDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return SweepScriptRun{}, err
		}
	}
	shim := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nenv > %q\nexec %q \"$@\"\n",
		filepath.Join(recordDir, "argv"), filepath.Join(recordDir, "env"), realCurl)
	if err := os.WriteFile(filepath.Join(shimDir, "curl"), []byte(shim), 0o700); err != nil { // #nosec G306 -- test shim must be executable.
		return SweepScriptRun{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command[0], command[1:]...) // #nosec G204 -- runs the chart-rendered command under test.
	cmd.Dir = workDir
	cmd.Env = []string{"PATH=" + shimDir + string(os.PathListSeparator) + os.Getenv("PATH")}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	run := SweepScriptRun{Stdout: stdout.String(), Stderr: stderr.String()}
	if argv, err := os.ReadFile(filepath.Join(recordDir, "argv")); err == nil { // #nosec G304 -- path inside the test temp dir.
		run.CurlArgv = string(argv)
	}
	if env, err := os.ReadFile(filepath.Join(recordDir, "env")); err == nil { // #nosec G304 -- path inside the test temp dir.
		run.CurlEnv = string(env)
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		run.ExitCode = exitErr.ExitCode()
	default:
		return run, fmt.Errorf("run sweep command: %w", runErr)
	}
	if ctx.Err() != nil {
		return run, fmt.Errorf("sweep command exceeded %s", timeout)
	}
	return run, nil
}

func sweepCommand(cronJob map[string]any) ([]string, error) {
	containers, ok := Path(cronJob, "spec", "jobTemplate", "spec", "template", "spec", "containers").([]any)
	if !ok || len(containers) == 0 {
		return nil, fmt.Errorf("cronjob has no containers")
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cronjob container is not an object")
	}
	raw, ok := container["command"].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("cronjob container has no command")
	}
	command := make([]string, 0, len(raw))
	for _, part := range raw {
		s, ok := part.(string)
		if !ok {
			return nil, fmt.Errorf("cronjob command contains a non-string element")
		}
		command = append(command, s)
	}
	return command, nil
}
