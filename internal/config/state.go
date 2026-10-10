package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	kdl "github.com/calico32/kdl-go"
)

// State is kubecom's per-context runtime state: values kubecom records for you as
// you use it, as opposed to the authored config.kdl (Config). It lives in its own
// per-context file (StatePath) for two reasons: state is per-context (a namespace
// name means different things across clusters), and keeping it separate lets
// kubecom rewrite it freely on every change (D90). The zero value is valid — a
// context with no state file starts on defaults. The file is KDL (D302):
//
//	version 1
//	namespace "kube-system"
//	resource "deployments" group="apps" version="v1" kind="Deployment" namespaced=#true
//	drill-owner "deployments" group="apps" version="v1" kind="Deployment" namespaced=#true namespace="web" name="api"
//	sort "AGE" ascending=#true
//	pin "certificates" group="cert-manager.io" version="v1" kind="Certificate" namespaced=#true
type State struct {
	// LastNamespace is the namespace scope last selected for this context, restored
	// as the initial watch scope on the next launch ("" = all namespaces). The
	// wiring leg (M2-11b-2) lets an explicit -n flag override it for that run.
	LastNamespace string

	// PinnedResources are the resource kinds the user reached for on this context and
	// kept — chiefly CRDs, which are too numerous to list in full and unreachable when
	// listed not at all (CRD-PIN-01/D193). They are folded into the menu exactly as the
	// authored `menu` entries are, so a pin and a hand-written entry render the same
	// row; the authored entry wins where both name a GVR. They live here
	// rather than in that file because kubecom writes them *for* you as you work, and
	// this is the file kubecom may rewrite freely (D90).
	PinnedResources []MenuResource

	// LastResource is the resource kind the user last browsed on this context, restored
	// as the opened table on the next launch and on every switch back (CTX-MEM-02/D240).
	// It is an *address* — a GVR and the display hints that go with it — never rows: rows
	// from a cluster kubecom is not connected to are stale the moment they are stored, so
	// the restore re-watches through the ordinary drill-in path instead (D240 pt 2).
	//
	// A pointer rather than a value because "never recorded" and "recorded" must be
	// distinguishable and `omitempty` does not omit a zero struct. Nil is the state of a
	// context that has only ever been browsed at the menu, and it restores nothing.
	LastResource *MenuResource

	// LastSortColumn is the name of the column the table was last sorted on.
	LastSortColumn string

	// LastSortAscending is true if the last sort was ascending.
	LastSortAscending bool

	// LastDrillOwner is the owner object a remembered drill-in scope was opened
	// from (CTX-MEM-04/D240 pt 6): the address of the *owner* the last pane's
	// children table was narrowed to. It is an address and nothing more — the
	// owner's kind plus namespace/name, exactly like LastResource — so a restore
	// can re-resolve the owner's child scope instead of landing on the plain
	// child table. Nil when the last pane was a plain table.
	LastDrillOwner *DrillOwner
}

// DrillOwner names the object a children drill-in scope was opened from (D165's
// ChildScope owner). It is the address a restore re-resolves the scope with: the
// owner's kind (as a MenuResource, so it addresses the kube layer like a pin) and
// the object ref that identifies it. Never rows, never the selector — the selector
// is re-derived by re-resolving the owner, which is the whole point (D240 pt 2/6).
type DrillOwner struct {
	// Resource is the owner's kind — group/version/resource plus the Kind display
	// hint, exactly the shape a pin or the remembered kind records.
	Resource MenuResource
	// Namespace is the owner's namespace ("" for a cluster-scoped owner like a
	// Node).
	Namespace string
	// Name is the owner object's name.
	Name string
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

// StateDir returns the directory holding the per-context state files:
// os.UserConfigDir()/kubecom/state, under the same kubecom config dir as the
// config files (D20), kept as its own folder so kubecom-owned runtime state never
// mixes with the authored config.
func StateDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state"), nil
}

// StatePath returns the state file path for a kubeconfig context:
// StateDir()/<sanitized-context>.kdl. The context name is sanitized into one safe
// path segment (stateFileName), so an arbitrary context name (path separators,
// GKE-style names) can never escape StateDir(). An empty context is an error:
// per-context state cannot be resolved without a context.
func StatePath(context string) (string, error) {
	if strings.TrimSpace(context) == "" {
		return "", errors.New("config: empty context; cannot resolve a per-context state file")
	}
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFileName(context)), nil
}

// stateFileName maps a kubeconfig context name to a safe single-segment filename
// ending in .kdl. Every character outside [A-Za-z0-9._-] becomes '_', so path
// separators and other unsafe characters can never escape StateDir() or split the
// path. It is intentionally lossy — two contexts differing only in sanitized
// characters share a file — the conservative tradeoff for the rare
// arbitrary-character context; the common cases (k3d-foo, gke_p_z_c, arn:aws:…
// becomes arn_aws_…) stay readable.
func stateFileName(context string) string {
	var b strings.Builder
	b.Grow(len(context) + len(".kdl"))
	for _, r := range context {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	b.WriteString(".kdl")
	return b.String()
}

// LoadStateFile reads the per-context state file at path. A missing file is not an
// error: it returns the zero State, since a context kubecom has never written state
// for simply starts on defaults (principle 3 — degrade, don't block).
func LoadStateFile(path string) (*State, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: reading state %s: %w", path, err)
	}
	return parseState(path, src)
}

// parseState reads a state file. Every node is checked — an unknown one, or a kind
// the kube layer could not address (no version, no resource), is an error naming
// its line — and the launcher degrades a state error to the zero state and a log
// line (principle 3): it costs the restore, never the launch.
func parseState(path string, src []byte) (*State, error) {
	doc, err := parse(path, src)
	if err != nil {
		return nil, err
	}
	var s State
	for _, n := range doc.Nodes[1:] {
		switch n.Name() {
		case "namespace":
			if s.LastNamespace, err = oneString(n); err != nil {
				return nil, err
			}
		case "resource":
			r, _, err := readResource(n)
			if err != nil {
				return nil, err
			}
			s.LastResource = &r
		case "drill-owner":
			r, extra, err := readResource(n, "namespace", "name")
			if err != nil {
				return nil, err
			}
			if extra["name"] == "" {
				return nil, fmt.Errorf("config: %s: drill-owner: name is required", n.Location())
			}
			s.LastDrillOwner = &DrillOwner{Resource: r, Namespace: extra["namespace"], Name: extra["name"]}
		case "sort":
			if s.LastSortColumn, err = oneString(n, "ascending"); err != nil {
				return nil, err
			}
			if v := n.Prop("ascending"); v.Kind() == kdl.Bool {
				s.LastSortAscending = v.Bool()
			}
		case "pin":
			r, _, err := readResource(n)
			if err != nil {
				return nil, err
			}
			s.PinnedResources = append(s.PinnedResources, r)
		default:
			return nil, fmt.Errorf("config: %s: %q is not a state node (namespace, resource, drill-owner, sort, pin)", n.Location(), n.Name())
		}
	}
	return &s, nil
}

// oneString reads a node of one string argument and no block; bools names the
// boolean properties it may carry.
func oneString(n *kdl.Node, bools ...string) (string, error) {
	if len(n.Arguments()) != 1 || n.Arg(0).Kind() != kdl.String || len(n.Children().Nodes) > 0 {
		return "", fmt.Errorf("config: %s: %s takes one string", n.Location(), n.Name())
	}
	for name, v := range n.Properties() {
		if !slices.Contains(bools, name) || v.Kind() != kdl.Bool {
			return "", fmt.Errorf("config: %s: %s has no property %q", n.Location(), n.Name(), name)
		}
	}
	return n.Arg(0).String(), nil
}

// Document is the state as a KDL document: what SaveFile writes, and what
// LoadStateFile reads back into an equal State. Fields at their zero value are
// left out.
func (s *State) Document() *kdl.Document {
	doc := kdl.NewDocument(kdl.NewNode("version", kdl.NewInt(fileVersion)))
	if s.LastNamespace != "" {
		doc.AddNode(kdl.NewNode("namespace", kdl.NewString(s.LastNamespace)))
	}
	if s.LastResource != nil {
		doc.AddNode(resourceNode("resource", *s.LastResource))
	}
	if o := s.LastDrillOwner; o != nil {
		n := resourceNode("drill-owner", o.Resource)
		if o.Namespace != "" {
			n.AddProperty("namespace", kdl.NewString(o.Namespace))
		}
		n.AddProperty("name", kdl.NewString(o.Name))
		doc.AddNode(n)
	}
	if s.LastSortColumn != "" {
		n := kdl.NewNode("sort", kdl.NewString(s.LastSortColumn))
		if s.LastSortAscending {
			n.AddProperty("ascending", kdl.NewBool(true))
		}
		doc.AddNode(n)
	}
	for _, r := range s.PinnedResources {
		doc.AddNode(resourceNode("pin", r))
	}
	return doc
}

// SaveFile persists the state to path, creating the parent dir (0o700) if needed
// and replacing any existing file atomically at 0o600 (atomicWriteFile). It runs
// whenever the picked namespace, a pin or the open kind changes, so it may run
// often; the atomic rename keeps a crash mid-write from truncating the live state.
func (s *State) SaveFile(path string) error {
	// One node per line however long, so the file reads as a list of facts.
	text, err := kdl.FormatToString(s.Document(), kdl.WithFormatMaxLineLen(1<<16))
	if err != nil {
		return fmt.Errorf("config: formatting state: %w", err)
	}
	return atomicWriteFile(path, []byte(text))
}
