package syn

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

type HookRun struct {
	Command  string
	Output   []string
	Duration time.Duration
	Err      error
}

// hookInfo is what the hook is told about the sync that woke it. A deploy
// script wants more than "something changed": which event, which commit it
// is now on, where it came from, and what actually moved - so it can restart
// only when the files it cares about did.
type hookInfo struct {
	Event   string
	Commit  string
	Prev    string
	Changed []string

	// Label is what to call this in the log. `syn restart` runs on the hook
	// machinery but was never set up as a hook, so calling it one in the
	// output sends people looking for a `syn hook` they never added.
	Label string
}

// what names the thing being run, for messages.
func (i hookInfo) what() string {
	if i.Label != "" {
		return i.Label
	}
	return i.Event + " hook"
}

func (e *Engine) runHooks(r *Repo, info hookInfo, hooks HookList) []HookRun {
	event := info.Event
	if len(hooks) == 0 {
		return nil
	}

	// A hook that named paths sits out a sync that did not touch them.
	wanted := make(HookList, 0, len(hooks))
	for _, h := range hooks {
		if h.Wants(info.Changed) {
			wanted = append(wanted, h)
			continue
		}
		Debug("%s: %s hook skipped, nothing under %s changed: %s",
			r.Name, event, strings.Join(h.Paths, ", "), h.Run)
	}
	if len(wanted) == 0 {
		return nil
	}

	if info.Label != "" {
		Info("%s: running the %s", r.Name, info.Label)
	} else {
		Info("%s: running %d %s hook(s)", r.Name, len(wanted), event)
	}
	timeout := time.Duration(r.Hooks.Timeout()) * time.Second

	var runs []HookRun
	for _, h := range wanted {
		cmd := h.Run
		run := e.runHook(r, info, cmd, timeout)
		runs = append(runs, run)

		if run.Err != nil {
			Errorf("%s: %s failed after %s: %s", r.Name, info.what(), run.Duration.Round(time.Millisecond), cmd)
			for _, line := range run.Output {
				Errorf("       %s", line)
			}
			break
		}

		Info("%s: %s ok in %s: %s", r.Name, info.what(), run.Duration.Round(time.Millisecond), cmd)
		for _, line := range run.Output {
			Info("       %s", line)
		}
	}
	return runs
}

func (e *Engine) runHook(r *Repo, info hookInfo, command string, timeout time.Duration) HookRun {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := shellCommand(ctx, command)
	cmd.Dir = r.Path
	cmd.Env = hookEnv(r, info)
	hideConsole(cmd)

	out, err := cmd.CombinedOutput()
	run := HookRun{
		Command:  command,
		Output:   tailLines(decodeConsole(out), 20),
		Duration: time.Since(start),
	}
	if ctx.Err() == context.DeadlineExceeded {
		run.Err = fmt.Errorf("timed out after %s", timeout)
		return run
	}
	run.Err = err
	return run
}

func hookEnv(r *Repo, info hookInfo) []string {
	env := append(os.Environ(),
		"SYN_REPO="+r.Name,
		"SYN_PATH="+r.Path,
		"SYN_EVENT="+info.Event,
		"SYN_COMMIT="+info.Commit,
		"SYN_PREV_COMMIT="+info.Prev,
		// One path per line, because a comma is a legal character in a
		// filename and a newline, in practice, is not. Read it with
		// something that splits on lines: while read f; do ...; done
		"SYN_CHANGED_FILES="+strings.Join(info.Changed, "\n"),
		fmt.Sprintf("SYN_CHANGED_COUNT=%d", len(info.Changed)),
	)
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
	cmds, ok := r.Hooks.For(event)
	if !ok {
		return fmt.Errorf("event must be after-pull, after-push or on-conflict")
	}
	if len(cmds) == 0 {
		return fmt.Errorf("%s has no %s hooks", r.Name, event)
	}
	// Run by hand there is no sync to describe, so the commit is simply
	// where the repository stands; nothing changed, and the hook can see
	// that from SYN_CHANGED_COUNT.
	head, _ := CurrentCommit(r.Path)
	for _, run := range e.runHooks(r, hookInfo{Event: event, Commit: head}, cmds) {
		if run.Err != nil {
			return fmt.Errorf("hook failed: %s: %w", run.Command, run.Err)
		}
	}
	return nil
}

// RunRestartNow runs the configured restart command without syncing.
func (e *Engine) RunRestartNow(r *Repo) []HookRun {
	if r.Restart == nil || r.Restart.Command == "" {
		return nil
	}
	head, _ := CurrentCommit(r.Path)
	// Run by hand there is nothing to match paths against, and no paths are
	// given, so it runs regardless - which is the point of trying it.
	return e.runHooks(r, hookInfo{Event: "after-pull", Commit: head, Label: "restart command"},
		HookList{{Run: r.Restart.Command}})
}
