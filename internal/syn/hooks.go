package syn

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type HookRun struct {
	Command  string
	Output   []string
	Duration time.Duration
	Err      error
}

func (e *Engine) runHooks(r *Repo, event string, commands []string) []HookRun {
	if len(commands) == 0 {
		return nil
	}

	Info("%s: running %d %s hook(s)", r.Name, len(commands), event)
	timeout := time.Duration(r.Hooks.Timeout()) * time.Second

	var runs []HookRun
	for _, cmd := range commands {
		run := e.runHook(r, cmd, timeout)
		runs = append(runs, run)

		if run.Err != nil {
			Errorf("%s: hook failed after %s: %s", r.Name, run.Duration.Round(time.Millisecond), cmd)
			for _, line := range run.Output {
				Errorf("       %s", line)
			}
			break
		}

		Info("%s: hook ok in %s: %s", r.Name, run.Duration.Round(time.Millisecond), cmd)
		for _, line := range run.Output {
			Info("       %s", line)
		}
	}
	return runs
}

func (e *Engine) runHook(r *Repo, command string, timeout time.Duration) HookRun {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = r.Path
	cmd.Env = append(hookEnv(r), "SYN_REPO="+r.Name, "SYN_PATH="+r.Path)
	hideConsole(cmd)

	out, err := cmd.CombinedOutput()
	run := HookRun{
		Command:  command,
		Output:   tailLines(string(out), 20),
		Duration: time.Since(start),
	}
	if ctx.Err() == context.DeadlineExceeded {
		run.Err = fmt.Errorf("timed out after %s", timeout)
		return run
	}
	run.Err = err
	return run
}

func hookEnv(r *Repo) []string {
	env := os.Environ()
	if branch, err := CurrentBranch(r.Path); err == nil {
		env = append(env, "SYN_BRANCH="+branch)
	}
	return env
}

func tailLines(s string, max int) []string {
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), " \t\r"); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > max {
		cut := len(lines) - max
		lines = append([]string{fmt.Sprintf("... %d earlier line(s) omitted", cut)}, lines[cut:]...)
	}
	return lines
}

func (e *Engine) RunHooksNow(r *Repo, event string) error {
	var cmds []string
	switch event {
	case "after-pull", "after_pull":
		cmds = r.Hooks.AfterPull
	case "after-push", "after_push":
		cmds = r.Hooks.AfterPush
	case "on-conflict", "on_conflict":
		cmds = r.Hooks.OnConflict
	default:
		return fmt.Errorf("event must be after-pull, after-push or on-conflict")
	}
	if len(cmds) == 0 {
		return fmt.Errorf("%s has no %s hooks", r.Name, event)
	}
	for _, run := range e.runHooks(r, event, cmds) {
		if run.Err != nil {
			return fmt.Errorf("hook failed: %s: %w", run.Command, run.Err)
		}
	}
	return nil
}
