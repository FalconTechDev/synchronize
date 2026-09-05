package syn

import (
	"encoding/json"
	"strings"
)

// Hook is one command and, optionally, the paths it cares about. A repository
// that holds a bot and its documentation should not restart the bot because a
// README moved.
type Hook struct {
	Run   string   `json:"run"`
	Paths []string `json:"paths,omitempty"`
}

// UnmarshalJSON accepts both shapes: the bare string hooks were written as
// before paths existed, and the object they need now. Configs already on disk
// keep working without being rewritten.
func (h *Hook) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		h.Run, h.Paths = s, nil
		return nil
	}
	type plain Hook
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*h = Hook(p)
	return nil
}

// MarshalJSON writes a hook without paths back as the plain string it came
// in as, so turning syn loose on an old config does not churn it.
func (h Hook) MarshalJSON() ([]byte, error) {
	if len(h.Paths) == 0 {
		return json.Marshal(h.Run)
	}
	type plain Hook
	return json.Marshal(plain(h))
}

func (h Hook) String() string {
	if len(h.Paths) == 0 {
		return h.Run
	}
	return h.Run + "  [paths: " + strings.Join(h.Paths, ", ") + "]"
}

// Wants says whether this hook should run for a sync that touched these
// files. A hook with no paths wants everything - that is the old behaviour,
// and the right default.
//
// A hook that asked for paths and got a change list it cannot see into is a
// different matter: on-conflict knows only the conflicted files, and a sync
// can report a commit without listing what went into it. Running the hook
// then is the safer mistake - a deploy that fires once too often beats one
// that silently stops firing.
func (h Hook) Wants(changed []string) bool {
	if len(h.Paths) == 0 || len(changed) == 0 {
		return true
	}
	for _, f := range changed {
		if matchesAny(f, h.Paths) {
			return true
		}
	}
	return false
}

// HookList is what the config holds per event.
type HookList []Hook

func (l HookList) Commands() []string {
	out := make([]string, 0, len(l))
	for _, h := range l {
		out = append(out, h.Run)
	}
	return out
}
