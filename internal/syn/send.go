package syn

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func (e *Engine) SendFiles(destRepo *Repo, srcPaths []string) error {
	sendDir := filepath.Join(destRepo.Path, "syn-send")
	if err := os.MkdirAll(sendDir, 0o755); err != nil {
		return err
	}

	for _, src := range srcPaths {
		dst := filepath.Join(sendDir, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", src, err)
		}
		Info("copied %s → %s", filepath.Base(src), dst)
	}

	
	if _, err := git(destRepo.Path, "add", "syn-send/"); err != nil {
		return err
	}
	msg := fmt.Sprintf("syn: send %d file(s) from %s at %s",
		len(srcPaths), hostname(), time.Now().Format("2006-01-02 15:04"))
	if _, err := gitCommit(destRepo.Path, msg); err != nil {
		return err
	}
	token := e.tokenFor(destRepo)
	if _, err := gitWithToken(destRepo.Path, token, "push", "--quiet", destRepo.Remote, destRepo.Branch); err != nil {
		return err
	}
	Info("pushed — files will arrive on the other machine shortly")
	return nil
}

func (e *Engine) SendViaNewTmp(creds *Credentials, cfg *Config, srcPaths []string) (string, error) {
	token := creds.Token("github.com")
	if token == "" {
		return "", fmt.Errorf("not logged in - run `syn auth login` first")
	}
	gh := NewGitHub(token)

	name := fmt.Sprintf("syn-send-%d", time.Now().Unix())
	Info("creating tmp repo %q on GitHub...", name)
	info, err := gh.CreateRepo(name, "syn one-shot file transfer", true)
	if err != nil {
		return "", err
	}

	localPath := filepath.Join(os.TempDir(), name)
	if err := CloneWithToken(info.CloneURL, localPath, token, "--quiet"); err != nil {
		return "", fmt.Errorf("clone failed: %w", err)
	}

	repo := &Repo{
		Name:       name,
		Path:       localPath,
		Remote:     "origin",
		Branch:     info.DefaultBranch,
		Enabled:    true,
		GithubSlug: info.FullName,
		Private:    true,
		Temp:       true,
	}
	cfg.Repos = append(cfg.Repos, *repo)
	if err := cfg.Save(); err != nil {
		return "", err
	}

	if err := e.SendFiles(repo, srcPaths); err != nil {
		return name, err
	}
	return name, nil
}

func (e *Engine) ReceiveFiles(repo *Repo, destDir string, stop <-chan struct{}) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	sendDir := filepath.Join(repo.Path, "syn-send")

	seen := map[string]bool{}
	
	if entries, err := os.ReadDir(sendDir); err == nil {
		for _, e := range entries {
			seen[e.Name()] = true
		}
	}

	Info("watching %s/syn-send/ for incoming files...", repo.Name)
	for {
		select {
		case <-stop:
			return nil
		case <-time.After(5 * time.Second):
		}

		
		e.Pull(repo)

		entries, err := os.ReadDir(sendDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || seen[entry.Name()] {
				continue
			}
			src := filepath.Join(sendDir, entry.Name())
			dst := filepath.Join(destDir, entry.Name())
			if err := copyFile(src, dst); err != nil {
				Warn("receive: copy %s: %v", entry.Name(), err)
				continue
			}
			seen[entry.Name()] = true
			Info("received: %s → %s", entry.Name(), dst)
			ToastNotify("synchronize: файл получен", entry.Name()+" → "+dst)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
