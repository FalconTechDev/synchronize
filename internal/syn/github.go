package syn

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	githubHost = "github.com"
	// DefaultClientID is the synchronize GitHub OAuth App.
	// Users who want their own app: `syn auth setup <client_id>`.
	DefaultClientID = ""
)

// GitHub is a thin GitHub API client that carries an auth token.
type GitHub struct {
	Token  string
	Client *http.Client
}

// NewGitHub returns a client. token may be "".
func NewGitHub(token string) *GitHub {
	return &GitHub{Token: token, Client: &http.Client{Timeout: 30 * time.Second}}
}

func (g *GitHub) do(method, path string, body io.Reader) (*http.Response, error) {
	if !strings.HasPrefix(path, "https://") {
		path = "https://api.github.com" + path
	}
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return g.Client.Do(req)
}

func (g *GitHub) get(path string, out any) error {
	resp, err := g.do("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("not authenticated (run `syn auth login`)")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub API %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// WhoAmI returns the authenticated user's login, or an error.
func (g *GitHub) WhoAmI() (string, string, error) {
	var u struct {
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := g.get("/user", &u); err != nil {
		return "", "", err
	}
	display := u.Login
	if u.Name != "" {
		display = u.Name + " (" + u.Login + ")"
	}
	return u.Login, display, nil
}

// RepoInfo fetches metadata about owner/repo from GitHub.
func (g *GitHub) RepoInfo(owner, repo string) (*GHRepo, error) {
	var r GHRepo
	if err := g.get("/repos/"+owner+"/"+repo, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRepos returns all repos the authenticated user can see (public + private).
func (g *GitHub) ListRepos(affiliation string) ([]GHRepo, error) {
	if affiliation == "" {
		affiliation = "owner,collaborator,organization_member"
	}
	var all []GHRepo
	page := 1
	for {
		var page_repos []GHRepo
		path := fmt.Sprintf("/user/repos?affiliation=%s&per_page=100&page=%d&sort=pushed",
			url.QueryEscape(affiliation), page)
		if err := g.get(path, &page_repos); err != nil {
			return nil, err
		}
		all = append(all, page_repos...)
		if len(page_repos) < 100 {
			break
		}
		page++
	}
	return all, nil
}

// GHRepo is a subset of the GitHub repo object we care about.
type GHRepo struct {
	FullName      string `json:"full_name"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
	Size          int    `json:"size"` // KB
}

// CloneURLWithAuth returns an HTTPS clone URL with the token embedded,
// suitable for use as a git remote (stored only in the local .git/config).
func (r *GHRepo) CloneURLWithAuth(token string) string {
	if token == "" {
		return r.CloneURL
	}
	u, err := url.Parse(r.CloneURL)
	if err != nil {
		return r.CloneURL
	}
	u.User = url.UserPassword("oauth2", token)
	return u.String()
}

// ---------------------------------------------------------------------------
// Device Flow (OAuth without a browser redirect)
// ---------------------------------------------------------------------------

// DeviceFlowStart kicks off the GitHub Device Flow and returns the result.
type DeviceFlowStart struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceFlowBegin starts the device flow for the given OAuth App client ID.
func DeviceFlowBegin(clientID string) (*DeviceFlowStart, error) {
	resp, err := http.PostForm("https://github.com/login/device/code", url.Values{
		"client_id": {clientID},
		"scope":     {"repo,read:user"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	vals, err := url.ParseQuery(string(b))
	if err != nil {
		return nil, err
	}
	if errMsg := vals.Get("error_description"); errMsg != "" {
		return nil, fmt.Errorf("GitHub: %s", errMsg)
	}
	interval := 5
	if v := vals.Get("interval"); v != "" {
		fmt.Sscanf(v, "%d", &interval)
	}
	expires := 900
	if v := vals.Get("expires_in"); v != "" {
		fmt.Sscanf(v, "%d", &expires)
	}
	return &DeviceFlowStart{
		DeviceCode:      vals.Get("device_code"),
		UserCode:        vals.Get("user_code"),
		VerificationURI: vals.Get("verification_uri"),
		ExpiresIn:       expires,
		Interval:        interval,
	}, nil
}

// DeviceFlowPoll polls GitHub until the user authorizes or the code expires.
// It prints status lines to stderr.
func DeviceFlowPoll(clientID string, d *DeviceFlowStart) (string, error) {
	deadline := time.Now().Add(time.Duration(d.ExpiresIn) * time.Second)
	interval := time.Duration(d.Interval) * time.Second
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		resp, err := http.PostForm("https://github.com/login/oauth/access_token", url.Values{
			"client_id":   {clientID},
			"device_code": {d.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		})
		if err != nil {
			return "", err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		vals, _ := url.ParseQuery(string(b))
		switch vals.Get("error") {
		case "authorization_pending":
			// Still waiting.
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token":
			return "", fmt.Errorf("code expired - run `syn auth login` again")
		case "access_denied":
			return "", fmt.Errorf("access denied")
		case "":
			if token := vals.Get("access_token"); token != "" {
				return token, nil
			}
		default:
			return "", fmt.Errorf("GitHub: %s - %s", vals.Get("error"), vals.Get("error_description"))
		}
	}
	return "", fmt.Errorf("timed out waiting for authorization")
}

// LoginPAT stores a Personal Access Token after a quick validation.
func LoginPAT(creds *Credentials, token string) (string, error) {
	gh := NewGitHub(token)
	login, display, err := gh.WhoAmI()
	if err != nil {
		return "", fmt.Errorf("token rejected by GitHub: %w", err)
	}
	creds.Set(githubHost, token)
	if err := creds.Save(); err != nil {
		return "", err
	}
	_ = login
	return display, nil
}

// LoginDeviceFlow runs the full device-flow auth and saves the resulting token.
// clientID is the GitHub OAuth App's client_id.
func LoginDeviceFlow(creds *Credentials, clientID string) (string, error) {
	d, err := DeviceFlowBegin(clientID)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "\nOpen this URL in your browser:\n  %s\n\nEnter code:  %s\n\nWaiting for authorization...\n",
		d.VerificationURI, d.UserCode)

	token, err := DeviceFlowPoll(clientID, d)
	if err != nil {
		return "", err
	}
	gh := NewGitHub(token)
	_, display, err := gh.WhoAmI()
	if err != nil {
		return "", fmt.Errorf("got a token but could not verify it: %w", err)
	}
	creds.Set(githubHost, token)
	if err := creds.Save(); err != nil {
		return "", err
	}
	return display, nil
}

// AuthStatus prints the current auth state to stdout and returns the login or "".
func AuthStatus(creds *Credentials) string {
	token := creds.Token(githubHost)
	if token == "" {
		return ""
	}
	gh := NewGitHub(token)
	_, display, err := gh.WhoAmI()
	if err != nil {
		return ""
	}
	return display
}

// promptLine reads a single line from stdin.
func promptLine(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	s := bufio.NewScanner(os.Stdin)
	if s.Scan() {
		return strings.TrimSpace(s.Text())
	}
	return ""
}

// CreateRepo creates a new repository on GitHub and returns its info.
func (g *GitHub) CreateRepo(name, description string, private bool) (*GHRepo, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"name":        name,
		"description": description,
		"private":     private,
		"auto_init":   true,
	})
	req, err := http.NewRequest("POST", "https://api.github.com/user/repos", strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API create repo: %s", resp.Status)
	}
	var r GHRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteRepo permanently deletes a GitHub repository. slug is "owner/repo".
func (g *GitHub) DeleteRepo(slug string) error {
	req, err := http.NewRequest("DELETE", "https://api.github.com/repos/"+slug, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil
	}
	return fmt.Errorf("GitHub API delete repo: %s", resp.Status)
}
