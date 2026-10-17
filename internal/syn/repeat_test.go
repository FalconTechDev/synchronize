package syn

import (
	"strings"
	"testing"
	"time"
)

// The complaint this answers: a repository whose folder is gone wrote the
// same line to the log every ten minutes, hundreds of times.
func TestAnUnchangedErrorIsNotRepeatedForever(t *testing.T) {
	clearRepeat("repo")
	const msg = "repo: FAILED - /gone no longer exists"

	line, worth := shouldReport("repo", msg)
	if !worth || line != msg {
		t.Fatalf("the first failure must be reported as-is: %q %v", line, worth)
	}

	// The second says so once, with how long it has been going on, so the
	// reader knows it is stuck rather than flapping.
	line, worth = shouldReport("repo", msg)
	if !worth {
		t.Fatal("the first repeat should be reported, to say it is persistent")
	}
	if !strings.Contains(line, "2 times") {
		t.Errorf("the repeat should say how many times: %q", line)
	}

	// And then it goes quiet.
	for i := 0; i < 50; i++ {
		if _, worth := shouldReport("repo", msg); worth {
			t.Fatalf("still logging after %d identical failures", i+3)
		}
	}
}

// Anything that changes is news and must come through at once.
func TestADifferentErrorIsAlwaysReported(t *testing.T) {
	clearRepeat("repo")
	shouldReport("repo", "first problem")
	shouldReport("repo", "first problem")
	for i := 0; i < 10; i++ {
		shouldReport("repo", "first problem")
	}
	line, worth := shouldReport("repo", "a different problem")
	if !worth {
		t.Fatal("a new error was swallowed because an old one was repeating")
	}
	if line != "a different problem" {
		t.Errorf("a new error should be reported plainly: %q", line)
	}
}

// After a success, the next failure is news again rather than a continuation.
func TestSuccessResetsTheCounter(t *testing.T) {
	clearRepeat("repo")
	shouldReport("repo", "problem")
	shouldReport("repo", "problem")
	clearRepeat("repo") // what a successful sync does

	line, worth := shouldReport("repo", "problem")
	if !worth || line != "problem" {
		t.Errorf("after a success the failure should read as new: %q %v", line, worth)
	}
}

// One repository going wrong must not silence another.
func TestRepositoriesAreCountedSeparately(t *testing.T) {
	clearRepeat("a")
	clearRepeat("b")
	shouldReport("a", "same text")
	shouldReport("a", "same text")
	if _, worth := shouldReport("b", "same text"); !worth {
		t.Error("a repeat on one repository silenced the first report on another")
	}
}

func TestRoughlyReadsLikeAPerson(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "less than a minute",
		5 * time.Minute:  "5 minutes",
		1 * time.Hour:    "1 hour",
		25 * time.Hour:   "1 day",
		72 * time.Hour:   "3 days",
	} {
		if got := roughly(d); got != want {
			t.Errorf("roughly(%v) = %q, want %q", d, got, want)
		}
	}
}
