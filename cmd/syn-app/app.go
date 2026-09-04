package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FalconTechDev/synchronize/internal/syn"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context

	mu     sync.Mutex
	cfg    *syn.Config
	engine *syn.Engine
	creds  *syn.Credentials

	watching bool
	watchStop chan struct{}
	busy     map[string]bool
}

func NewApp() *App {
	return &App{busy: map[string]bool{}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.reload()
	go a.logPump()
}

func (a *App) reload() {
	cfg, err := syn.LoadConfig()
	if err != nil {
		a.emit("error", err.Error())
		return
	}
	a.mu.Lock()
	a.cfg = cfg
	a.engine = syn.NewEngine(cfg)
	a.creds = syn.LoadCredentials()
	syn.FallbackIdentity = cfg.Identity
	a.mu.Unlock()
}

func (a *App) emit(event string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, event, data)
	}
}

// ------------------------------------------------------------------ types

type RepoView struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Slug      string `json:"slug"`
	Branch    string `json:"branch"`
	Remote    string `json:"remote"`
	Tracking  string `json:"tracking"`
	Ahead     int    `json:"ahead"`
	Behind    int    `json:"behind"`
	Dirty     bool   `json:"dirty"`
	Changed   int    `json:"changed"`
	Enabled   bool   `json:"enabled"`
	Private   bool   `json:"private"`
	Interval  int    `json:"interval"`
	LastSync  string `json:"lastSync"`
	Conflict  string `json:"conflict"`
	Busy      bool   `json:"busy"`
	Err       string `json:"error"`
}

type AppState struct {
	Repos     []RepoView `json:"repos"`
	Account   string     `json:"account"`
	LoggedIn  bool       `json:"loggedIn"`
	Watching  bool       `json:"watching"`
	Version   string     `json:"version"`
	Identity  string     `json:"identity"`
	ConfigDir string     `json:"configDir"`
}

// ------------------------------------------------------------------ queries

func (a *App) GetState(fetch bool) AppState {
	a.mu.Lock()
	cfg, engine := a.cfg, a.engine
	watching := a.watching
	busy := make(map[string]bool, len(a.busy))
	for k, v := range a.busy {
		busy[k] = v
	}
	a.mu.Unlock()

	st := AppState{
		Version:   Version,
		Watching:  watching,
		ConfigDir: syn.Home(),
	}
	if cfg == nil {
		return st
	}
	if !cfg.Identity.IsZero() {
		st.Identity = fmt.Sprintf("%s <%s>", cfg.Identity.Name, cfg.Identity.Email)
	}
	if a.creds != nil && a.creds.Token("github.com") != "" {
		st.LoggedIn = true
		st.Account = cfg.Identity.Name
	}

	for i := range cfg.Repos {
		r := &cfg.Repos[i]
		s := engine.Status(r, fetch)

		v := RepoView{
			Name:     r.Name,
			Path:     r.Path,
			Slug:     r.GithubSlug,
			Branch:   s.Branch,
			Remote:   r.Remote,
			Ahead:    s.Ahead,
			Behind:   s.Behind,
			Dirty:    s.Dirty,
			Changed:  len(s.Changed),
			Enabled:  r.Enabled,
			Private:  r.Private,
			Interval: cfg.IntervalFor(r),
			Busy:     busy[r.Name],
		}
		if s.Branch != "" && r.Remote != "" {
			v.Tracking = r.Remote + "/" + s.Branch
		}
		if !s.LastSync.IsZero() {
			v.LastSync = humanAgo(s.LastSync)
		}
		if s.Conflict != nil {
			v.Conflict = s.Conflict.Reason
		}
		if s.Err != nil {
			v.Err = s.Err.Error()
		}
		st.Repos = append(st.Repos, v)
	}
	sort.Slice(st.Repos, func(i, j int) bool { return st.Repos[i].Name < st.Repos[j].Name })
	return st
}

func humanAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 10*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ------------------------------------------------------------------ actions

type OpResult struct {
	OK      bool     `json:"ok"`
	Summary string   `json:"summary"`
	Lines   []string `json:"lines"`
	Error   string   `json:"error"`
}

func (a *App) run(name string, op func(*syn.Repo) syn.Result) OpResult {
	a.mu.Lock()
	repo := a.cfg.Find(name)
	if repo == nil {
		a.mu.Unlock()
		return OpResult{Error: fmt.Sprintf("repo %q is not registered", name)}
	}
	a.busy[name] = true
	a.mu.Unlock()

	a.emit("busy", name)
	res := op(repo)

	a.mu.Lock()
	delete(a.busy, name)
	a.mu.Unlock()

	out := OpResult{OK: res.OK(), Summary: res.Summary()}
	out.Lines = append(out.Lines, res.PulledLog...)
	out.Lines = append(out.Lines, res.PushedLog...)
	if res.Err != nil {
		out.Error = res.Err.Error()
	}
	if res.Conflict != nil {
		out.Error = res.Conflict.Reason
	}
	a.emit("changed", name)
	return out
}

func (a *App) Pull(name string) OpResult { return a.run(name, a.engine.Pull) }
func (a *App) Push(name string) OpResult { return a.run(name, a.engine.Push) }
func (a *App) Sync(name string) OpResult { return a.run(name, a.engine.Sync) }

func (a *App) SyncAll() []OpResult {
	a.mu.Lock()
	repos := a.cfg.EnabledRepos()
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	a.mu.Unlock()

	out := make([]OpResult, 0, len(names))
	for _, n := range names {
		out = append(out, a.Sync(n))
	}
	return out
}

func (a *App) SetEnabled(name string, on bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.cfg.Find(name)
	if r == nil {
		return fmt.Errorf("repo %q is not registered", name)
	}
	r.Enabled = on
	return a.cfg.Save()
}

func (a *App) SetInterval(name string, minutes int) error {
	if minutes <= 0 {
		return fmt.Errorf("interval must be at least 1 minute")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.cfg.Find(name)
	if r == nil {
		return fmt.Errorf("repo %q is not registered", name)
	}
	r.IntervalMinutes = minutes
	return a.cfg.Save()
}

func (a *App) RemoveRepo(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.cfg.Repos {
		if a.cfg.Repos[i].Name == name {
			a.cfg.Repos = append(a.cfg.Repos[:i], a.cfg.Repos[i+1:]...)
			return a.cfg.Save()
		}
	}
	return fmt.Errorf("repo %q is not registered", name)
}

func (a *App) Diff(name string) (string, error) {
	a.mu.Lock()
	r := a.cfg.Find(name)
	a.mu.Unlock()
	if r == nil {
		return "", fmt.Errorf("repo %q is not registered", name)
	}
	branch := r.Branch
	if branch == "" {
		branch, _ = syn.CurrentBranch(r.Path)
	}
	return syn.Diff(r.Path, r.Remote, branch)
}

type RepoDetail struct {
	RepoView
	Commits  []syn.Commit     `json:"commits"`
	Incoming []string         `json:"incoming"`
	Outgoing []string         `json:"outgoing"`
	Files    []syn.FileChange `json:"files"`
}

func (a *App) GetDetail(name string) (RepoDetail, error) {
	a.mu.Lock()
	cfg, engine := a.cfg, a.engine
	a.mu.Unlock()

	r := cfg.Find(name)
	if r == nil {
		return RepoDetail{}, fmt.Errorf("repo %q is not registered", name)
	}
	s := engine.Status(r, false)

	d := RepoDetail{RepoView: RepoView{
		Name:     r.Name,
		Path:     r.Path,
		Slug:     r.GithubSlug,
		Branch:   s.Branch,
		Remote:   r.Remote,
		Ahead:    s.Ahead,
		Behind:   s.Behind,
		Dirty:    s.Dirty,
		Changed:  len(s.Changed),
		Enabled:  r.Enabled,
		Private:  r.Private,
		Interval: cfg.IntervalFor(r),
	}}
	if s.Branch != "" && r.Remote != "" {
		d.Tracking = r.Remote + "/" + s.Branch
	}
	if !s.LastSync.IsZero() {
		d.LastSync = humanAgo(s.LastSync)
	}
	if s.Conflict != nil {
		d.Conflict = s.Conflict.Reason
	}
	if s.Err != nil {
		d.Err = s.Err.Error()
		return d, nil
	}

	d.Commits = syn.RecentCommits(r.Path, 12)
	d.Files = syn.ChangedFilesDetailed(r.Path)
	if d.Tracking != "" {
		d.Incoming = syn.CommitLog(r.Path, "HEAD", d.Tracking, 10)
		d.Outgoing = syn.CommitLog(r.Path, d.Tracking, "HEAD", 10)
	}
	return d, nil
}

func (a *App) OpenFolder(path string) {
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(path))
}

func (a *App) OpenOnGitHub(slug string) {
	if slug == "" {
		return
	}
	runtime.BrowserOpenURL(a.ctx, "https://github.com/"+slug)
}

// ------------------------------------------------------------------ adding repos

func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose a git repository",
	})
}

func (a *App) AddLocal(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !syn.IsRepo(abs) {
		return fmt.Errorf("%s is not a git repository", abs)
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	name := filepath.Base(abs)
	if a.cfg.Find(name) != nil {
		return fmt.Errorf("%q is already registered", name)
	}
	branch, _ := syn.CurrentBranch(abs)
	a.cfg.Repos = append(a.cfg.Repos, syn.Repo{
		Name: name, Path: abs, Remote: "origin", Branch: branch, Enabled: true,
	})
	return a.cfg.Save()
}

func (a *App) ListGitHubRepos() ([]string, error) {
	a.mu.Lock()
	token := a.creds.Token("github.com")
	a.mu.Unlock()
	if token == "" {
		return nil, fmt.Errorf("not signed in to GitHub")
	}
	repos, err := syn.NewGitHub(token).ListRepos("owner")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		out = append(out, r.FullName)
	}
	return out, nil
}

func (a *App) CloneAndAdd(slug, dest string) error {
	a.mu.Lock()
	token := a.creds.Token("github.com")
	a.mu.Unlock()
	if token == "" {
		return fmt.Errorf("not signed in to GitHub")
	}
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected owner/repo, got %q", slug)
	}

	gh := syn.NewGitHub(token)
	info, err := gh.RepoInfo(parts[0], parts[1])
	if err != nil {
		return err
	}
	if dest == "" {
		home, _ := os.UserHomeDir()
		dest = filepath.Join(home, parts[1])
	}
	a.emit("log", fmt.Sprintf("cloning %s into %s ...", slug, dest))
	if err := syn.CloneWithToken(info.CloneURL, dest, token, "--quiet"); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Repos = append(a.cfg.Repos, syn.Repo{
		Name:       parts[1],
		Path:       dest,
		Remote:     "origin",
		Branch:     info.DefaultBranch,
		Enabled:    true,
		GithubSlug: info.FullName,
		Private:    info.Private,
	})
	return a.cfg.Save()
}

// ------------------------------------------------------------------ auth

func (a *App) SignIn(pat string) (string, error) {
	pat = strings.TrimSpace(pat)
	if pat == "" {
		return "", fmt.Errorf("paste a Personal Access Token first")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	display, err := syn.LoginPAT(a.creds, a.cfg, pat)
	if err != nil {
		return "", err
	}
	return display, nil
}

func (a *App) SignOut() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.creds.Delete("github.com")
	return a.creds.Save()
}

// ------------------------------------------------------------------ watch

func (a *App) ToggleWatch() bool {
	a.mu.Lock()
	if a.watching {
		close(a.watchStop)
		a.watching = false
		a.mu.Unlock()
		a.emit("log", "watcher stopped")
		return false
	}
	stop := make(chan struct{})
	a.watchStop = stop
	a.watching = true
	repos := a.cfg.EnabledRepos()
	engine := a.engine
	a.mu.Unlock()

	if len(repos) == 0 {
		a.mu.Lock()
		a.watching = false
		a.mu.Unlock()
		a.emit("log", "nothing to watch - every repository is paused")
		return false
	}

	go func() {
		engine.WatchUntil(repos, stop, func(res syn.Result) {
			a.emit("changed", res.Repo)
		})
	}()
	a.emit("log", fmt.Sprintf("watching %d repo(s)", len(repos)))
	return true
}

// WatchedNames reports which repositories the running watcher covers.
func (a *App) WatchedNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.watching {
		return nil
	}
	var out []string
	for _, r := range a.cfg.EnabledRepos() {
		out = append(out, r.Name)
	}
	return out
}

// ------------------------------------------------------------------ log

func (a *App) logPump() {
	stop := make(chan struct{})
	lines := make(chan string, 128)
	go syn.TailLines(lines, stop)
	for line := range lines {
		a.emit("log", line)
	}
}

func (a *App) RecentLog(n int) []string {
	return strings.Split(syn.TailLog(n), "\n")
}
