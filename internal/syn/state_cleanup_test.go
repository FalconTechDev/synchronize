package syn

import (
	"testing"
	"time"
)

// Removing a repository has to take its recorded state with it. Left behind,
// a conflict record attaches itself to the next repository registered under
// the same name - and syncing now refuses while one is on record, so that
// repository would be stuck from the moment it was added.
func TestRemovingARepositoryClearsItsState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)

	st := LoadState()
	st.Conflicts["bot"] = Conflict{Repo: "bot", Files: []string{"a.txt"}, At: time.Now()}
	st.LastSync["bot"] = time.Now()
	st.LastPrune["bot"] = time.Now()
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	ForgetRepoState("bot")

	again := LoadState()
	if _, ok := again.Conflicts["bot"]; ok {
		t.Error("the conflict record outlived the repository")
	}
	if _, ok := again.LastSync["bot"]; ok {
		t.Error("the last-sync time outlived the repository")
	}
	if _, ok := again.LastPrune["bot"]; ok {
		t.Error("the last-prune time outlived the repository")
	}
}

// Renaming must carry the state across, or a conflict is silently forgotten
// and the repository starts syncing again with the markers still in it.
func TestRenamingARepositoryCarriesItsState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNCHRONIZE_HOME", dir)

	when := time.Now().Add(-time.Hour).Round(time.Second)
	st := LoadState()
	st.Conflicts["old"] = Conflict{Repo: "old", Files: []string{"a.txt"}, At: when}
	st.LastSync["old"] = when
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	RenameRepoState("old", "new")

	again := LoadState()
	if _, ok := again.Conflicts["old"]; ok {
		t.Error("the old name still has a conflict record")
	}
	c, ok := again.Conflicts["new"]
	if !ok {
		t.Fatal("the conflict did not follow the rename - it would be silently forgotten")
	}
	if c.Repo != "new" {
		t.Errorf("the record still calls itself %q", c.Repo)
	}
	if got, ok := again.LastSync["new"]; !ok || !got.Equal(when) {
		t.Errorf("last sync did not follow the rename: %v", got)
	}
}
