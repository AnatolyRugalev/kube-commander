package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// State is kubecom's per-context runtime state: values kubecom records for you as
// you use it, as opposed to the user-authored config.yaml (Config) and per-context
// menu files (MenuConfig). It lives in its own per-context file (StatePath) for two
// reasons: state is per-context (a namespace name means different things across
// clusters), and keeping it separate lets kubecom rewrite it freely on every change
// without ever reformatting or clobbering the comments in a hand-edited config file
// (D90). The zero value is valid — a context with no state file starts on defaults.
type State struct {
	// LastNamespace is the namespace scope last selected for this context, restored
	// as the initial watch scope on the next launch ("" = all namespaces). The
	// wiring leg (M2-11b-2) lets an explicit -n flag override it for that run.
	LastNamespace string `json:"lastNamespace,omitempty"`

	// PinnedResources are the resource kinds the user reached for on this context and
	// kept — chiefly CRDs, which are too numerous to list in full and unreachable when
	// listed not at all (CRD-PIN-01/D193). They are folded into the menu exactly as the
	// authored menus/<context>.yaml entries are, so a pin and a hand-written entry
	// render the same row; the authored file wins where both name a GVR. They live here
	// rather than in that file because kubecom writes them *for* you as you work, and
	// this is the file kubecom may rewrite freely (D90).
	PinnedResources []MenuResource `json:"pinnedResources,omitempty"`

	// LastResource is the resource kind the user last browsed on this context, restored
	// as the opened table on the next launch and on every switch back (CTX-MEM-02/D240).
	// It is an *address* — a GVR and the display hints that go with it — never rows: rows
	// from a cluster kubecom is not connected to are stale the moment they are stored, so
	// the restore re-watches through the ordinary drill-in path instead (D240 pt 2).
	//
	// A pointer rather than a value because "never recorded" and "recorded" must be
	// distinguishable and `omitempty` does not omit a zero struct. Nil is the state of a
	// context that has only ever been browsed at the menu, and it restores nothing.
	LastResource *MenuResource `json:"lastResource,omitempty"`
}

// Pin adds r to the context's pinned kinds, returning false when the GVR is already
// pinned (no duplicate row, no needless write). It is the model half of the pin
// gesture: the caller persists with SaveFile only when this returns true.
func (s *State) Pin(r MenuResource) bool {
	for i := range s.PinnedResources {
		if sameGVR(s.PinnedResources[i], r) {
			return false
		}
	}
	s.PinnedResources = append(s.PinnedResources, r)
	return true
}

// Unpin removes the pinned kind with this group/version/resource, returning false
// when nothing was pinned for it. The key is the GVR rather than the whole entry so
// a row can be unpinned from what the menu knows about it, without reconstructing
// the display hints the pin was stored with.
func (s *State) Unpin(group, version, resource string) bool {
	for i := range s.PinnedResources {
		p := s.PinnedResources[i]
		if p.Group == group && p.Version == version && p.Resource == resource {
			s.PinnedResources = append(s.PinnedResources[:i], s.PinnedResources[i+1:]...)
			return true
		}
	}
	return false
}

// sameGVR reports whether two menu entries address the same API resource — the
// identity the menu itself dedupes on (menu.AddExtras keys off the GVR), so pins
// agree with it rather than treating two spellings of one kind as two rows.
func sameGVR(a, b MenuResource) bool {
	return a.Group == b.Group && a.Version == b.Version && a.Resource == b.Resource
}

// StateDir returns the directory holding the per-context state files:
// os.UserConfigDir()/kubecom/state — a sibling of the menus dir (MenuDir) under the
// same kubecom config dir (D20), kept as its own folder so kubecom-owned runtime
// state never mixes with the user-authored config.yaml or menu files.
func StateDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locating user config dir: %w", err)
	}
	return filepath.Join(dir, "kubecom", "state"), nil
}

// StatePath returns the state file path for a kubeconfig context:
// StateDir()/<sanitized-context>.yaml. It reuses the same context-name sanitization
// as the per-context menu file (menuFileName), so an arbitrary context name (path
// separators, GKE-style names) can never escape StateDir(). An empty context is an
// error: per-context state cannot be resolved without a context.
func StatePath(context string) (string, error) {
	if strings.TrimSpace(context) == "" {
		return "", errors.New("config: empty context; cannot resolve a per-context state file")
	}
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, menuFileName(context)), nil
}

// LoadState decodes a State from r. Unknown fields are rejected so a typo fails
// loudly instead of being silently ignored (mirrors Load / LoadMenu).
func LoadState(r io.Reader) (*State, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("config: reading state: %w", err)
	}
	return parseState(data)
}

// LoadStateFile reads the per-context state file at path. A missing file is not an
// error: it returns the zero State, since a context kubecom has never written state
// for simply starts on defaults (principle 3 — degrade, don't block).
func LoadStateFile(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: reading state %s: %w", path, err)
	}
	return parseState(data)
}

func parseState(data []byte) (*State, error) {
	var s State
	if len(data) > 0 {
		if err := yaml.UnmarshalStrict(data, &s); err != nil {
			return nil, fmt.Errorf("config: parsing state YAML: %w", err)
		}
	}
	// Pinned kinds are validated exactly as the authored menu file's entries are: a
	// pin the kube layer could not address is a broken row, whichever file it came
	// from. The launcher degrades a state-file error to the zero state and logs it
	// (principle 3), so an unaddressable pin costs the recorded namespace but never
	// the launch.
	if err := validateMenuResources("state pinnedResources", s.PinnedResources); err != nil {
		return nil, err
	}
	// The remembered kind is validated the same way and for the same reason: it is
	// addressed by the kube layer exactly as a pin is, so a version- or resource-less
	// entry is a broken address whichever field it came from. It degrades no worse
	// either — the launcher turns a state-file error into the zero state and a log
	// line, which costs the restore and the namespace but never the launch.
	if s.LastResource != nil {
		if err := validateMenuResources("state lastResource", []MenuResource{*s.LastResource}); err != nil {
			return nil, err
		}
	}
	return &s, nil
}

// Save writes the state as plain YAML to w — the write-back counterpart of
// LoadState (what Save emits, LoadState reads back equal; an all-zero State is a
// single "{}\n" thanks to the omitempty tags). It does not touch the filesystem —
// see SaveFile.
func (s *State) Save(w io.Writer) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("config: marshalling state YAML: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("config: writing state: %w", err)
	}
	return nil
}

// SaveFile persists the state to path, creating the parent dir (0o700) if needed
// and replacing any existing file atomically at 0o600 — the same crash-safe,
// private write Config.SaveFile uses (atomicWriteFile). The wiring leg calls this
// whenever the picked namespace changes, so it may run often; the atomic rename
// keeps a crash mid-write from truncating the live state.
func (s *State) SaveFile(path string) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("config: marshalling state YAML: %w", err)
	}
	return atomicWriteFile(path, data)
}
