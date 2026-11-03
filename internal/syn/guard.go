package syn

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Two things an automatic commit should not take without being asked.
//
// syn commits everything that changed, every ten minutes, and pushes it. That
// is the point of it, and it is also how a new .env ends up on a public
// repository, or a 300 MB video ends up in a commit GitHub then refuses -
// after which every later sync fails the same way until somebody notices.
//
// Both rules are deliberately narrow, so they get in the way of nobody who
// meant it:
//
//   - a secret is only held back while it is new and untracked. Once you have
//     staged or committed it yourself the decision is made, and syn carries it.
//   - a large file is only held back above what the remote would reject anyway.

// secretNames are file names that are almost never meant to be published.
var secretNames = []string{
	".env", ".env.*", "*.pem", "*.key", "*.p12", "*.pfx", "*.kdbx",
	"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519",
	".npmrc", ".pypirc", ".netrc", "credentials.json",
}

// notSecret are the endings that say a file is a template, not the real thing.
var notSecret = []string{".example", ".sample", ".template", ".dist", ".pub"}

func looksLikeSecret(file string) bool {
	base := strings.ToLower(path.Base(filepath.ToSlash(file)))
	for _, tail := range notSecret {
		if strings.HasSuffix(base, tail) {
			return false
		}
	}
	for _, pattern := range secretNames {
		if ok, _ := path.Match(pattern, base); ok {
			return true
		}
	}
	return false
}

// defaultMaxFileMB is just under GitHub's hard limit of 100 MiB per file.
const defaultMaxFileMB = 95

func maxFileBytes() int64 {
	if v := os.Getenv("SYN_MAX_FILE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return int64(n) << 20
		}
	}
	return int64(defaultMaxFileMB) << 20
}

// usesLFS says whether the repository stores big files through Git LFS. There
// the file on disk is large but what gets committed is a small pointer, so
// the size rule would hold back exactly the files that are working properly.
func usesLFS(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	return err == nil && strings.Contains(string(b), "filter=lfs")
}

// HeldFile is a file the guard stepped over, and the reason.
type HeldFile struct {
	File FileChange
	Why  string
}

// guard splits the files an automatic commit was about to take into those it
// may take and those it should leave for a person to decide about.
func guard(r *Repo, files []FileChange) (take []FileChange, held []HeldFile) {
	lfs := usesLFS(r.Path)
	limit := maxFileBytes()

	for _, f := range files {
		if f.Status == "new" && !r.SecretGuardOff && looksLikeSecret(f.Path) {
			held = append(held, HeldFile{f, "looks like a secret"})
			continue
		}
		if !lfs && f.Status != "deleted" {
			if info, err := os.Stat(filepath.Join(r.Path, filepath.FromSlash(f.Path))); err == nil &&
				!info.IsDir() && info.Size() > limit {
				held = append(held, HeldFile{f, fmt.Sprintf("%d MB, more than the remote accepts", info.Size()>>20)})
				continue
			}
		}
		take = append(take, f)
	}
	return take, held
}

// explainHeld tells the person what was left out and how to overrule it. Said
// once per file rather than every ten minutes: the same sentence on every
// tick is how a warning gets ignored.
func explainHeld(r *Repo, held []HeldFile) {
	for _, h := range held {
		key := r.Name + "\x00" + h.File.Path
		line := fmt.Sprintf("%s: not committing %s - %s\n"+
			"       to commit it anyway, do it yourself once:  git add -f %q && git commit",
			r.Name, h.File.Path, h.Why, h.File.Path)
		if out, worth := shouldReport(key, line); worth {
			Warn("%s", out)
		}
	}
}
