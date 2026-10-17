package syn

import (
	"sync"
	"time"
)

// Saying the same thing every ten minutes is not reporting, it is noise.
//
// A repository whose folder has gone - unplugged drive, moved project - fails
// identically on every tick, for days. The log fills with one error repeated
// hundreds of times, which buries everything else and tells the reader
// nothing they did not know after the first line.
//
// So an error that has not changed is said once, then again only when it is
// worth saying again: at the first repeat, and after that once an hour, each
// time with how long it has been going on. Anything that changes is reported
// immediately, because that is news.

type repeatedError struct {
	message string
	first   time.Time
	last    time.Time
	count   int
}

var (
	repeatMu  sync.Mutex
	repeating = map[string]*repeatedError{}
)

// the interval between repeats of an unchanged message.
const repeatEvery = time.Hour

// shouldReport decides whether this error from this repository is worth a log
// line, and returns the line to use. A different message always reports.
func shouldReport(key, message string) (string, bool) {
	repeatMu.Lock()
	defer repeatMu.Unlock()

	now := time.Now()
	prev, seen := repeating[key]
	if !seen || prev.message != message {
		repeating[key] = &repeatedError{message: message, first: now, last: now, count: 1}
		return message, true
	}

	prev.count++
	if now.Sub(prev.last) < repeatEvery && prev.count != 2 {
		return "", false
	}
	prev.last = now
	return message + repeatSuffix(prev.count, now.Sub(prev.first)), true
}

// clearRepeat forgets a repository's last error, so the next failure is
// reported as the news it is rather than silently counted.
func clearRepeat(key string) {
	repeatMu.Lock()
	defer repeatMu.Unlock()
	delete(repeating, key)
}

func repeatSuffix(count int, since time.Duration) string {
	return "\n       (the same thing " + plural(count, "time") + " now, since " +
		roughly(since) + " ago - nothing new will be logged for a while)"
}

// roughly describes a span the way a person would say it.
func roughly(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		return plural(int(d.Hours()/24), "day")
	}
}
