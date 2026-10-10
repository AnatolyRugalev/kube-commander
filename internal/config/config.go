package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	kdl "github.com/calico32/kdl-go"
)

const (
	// AuthoredName is the authored file's name in Dir: the person's config, which
	// kubecom never writes.
	AuthoredName = "config.kdl"
	// OverlayName is the overlay's name in Dir: what kubecom wrote, merged on top
	// of the authored file.
	OverlayName = "config.overlay.kdl"

	// keymapDocument is the key bindings (D303), handed whole to the keymap; the
	// overlay may hold one too.
	keymapDocument = "keymap"
	// menuDocument is a per-context resource menu, read from the authored file only.
	menuDocument = "menu"

	// fileVersion is the one version this build reads and writes: a newer file is
	// refused, naming it.
	fileVersion = 1
)

// Setting is one key a config file may set, with the kind of value it takes.
type Setting struct {
	Key  string
	Kind kdl.ValueKind
}

// Settings are every key kubecom reads from the files. A key that is not here is a
// load error naming its line, so a typo fails loudly instead of being silently
// ignored. The keymap and the menus are documents, not settings.
var Settings = []Setting{
	// The built-in color theme to render with (tui/styles.ByName: lenient about case
	// and surrounding space, never fuzzy). An unknown name degrades to the default
	// theme with a start-up notice (principle 3, D169).
	{Key: "theme", Kind: kdl.String},
}

// lookupSetting is the setting named key.
func lookupSetting(key string) (Setting, bool) {
	i := slices.IndexFunc(Settings, func(s Setting) bool { return s.Key == key })
	if i < 0 {
		return Setting{}, false
	}
	return Settings[i], true
}

// Dir is kubecom's configuration directory, os.UserConfigDir()/kubecom (D20):
// ~/.config/kubecom on Linux, ~/Library/Application Support/kubecom on macOS. It
// is kept apart from ~/.kube and from the discovery cache.
func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locating user config dir: %w", err)
	}
	return filepath.Join(dir, "kubecom"), nil
}

// Config is the two files, read: the authored values and the overlay's — the
// overlay kept as its parsed document too, so a write changes only what it must.
//
// A Config has one owner and is not safe for concurrent use.
type Config struct {
	authoredPath, overlayPath string
	authored                  map[string]leaf
	overlay                   *kdl.Document // nil until the overlay exists
	over                      map[string]leaf
	// keymaps are the `keymap` documents of the authored file and the overlay, nil
	// where a file has none.
	keymap, overKeymap *kdl.Node
	// menus are the authored file's `menu` documents, in file order.
	menus []*kdl.Node
}

// Load reads config.kdl and config.overlay.kdl from dir. A missing file is an
// empty one. A file that cannot be read, is not KDL (the error names its line and
// column), does not start with `version 1`, sets a key kubecom does not read or
// gives one the wrong kind of value is an error naming the file, and no Config.
func Load(dir string) (*Config, error) {
	c := &Config{
		authoredPath: filepath.Join(dir, AuthoredName),
		overlayPath:  filepath.Join(dir, OverlayName),
	}
	authored, leaves, err := readFile(c.authoredPath)
	if err != nil {
		return nil, err
	}
	c.authored = leaves
	if c.overlay, c.over, err = readFile(c.overlayPath); err != nil {
		return nil, err
	}
	c.keymap = lastNode(authored, keymapDocument)
	c.overKeymap = lastNode(c.overlay, keymapDocument)
	if c.overlay != nil {
		if n := lastNode(c.overlay, menuDocument); n != nil {
			return nil, fmt.Errorf("config: %s: the overlay holds no menu", n.Location())
		}
	}
	if authored != nil {
		for _, n := range authored.Nodes[1:] {
			if n.Name() == menuDocument {
				c.menus = append(c.menus, n)
			}
		}
		if _, err := c.readMenus(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// AuthoredPath is config.kdl's path; OverlayPath the overlay's.
func (c *Config) AuthoredPath() string { return c.authoredPath }

// OverlayPath is config.overlay.kdl's path.
func (c *Config) OverlayPath() string { return c.overlayPath }

// lastNode is a file's last top-level node named name (KDL's rightmost-wins), or
// nil.
func lastNode(doc *kdl.Document, name string) *kdl.Node {
	if doc == nil {
		return nil
	}
	var out *kdl.Node
	for _, n := range doc.Nodes[1:] {
		if n.Name() == name {
			out = n
		}
	}
	return out
}

// isDocument reports whether a top-level node's name is a document's.
func isDocument(name string) bool { return name == keymapDocument || name == menuDocument }

// Keymap is the children of the authored file's `keymap` document, as parsed (with
// their lines), and false when it has none.
func (c *Config) Keymap() ([]*kdl.Node, bool) { return children(c.keymap) }

// OverlayKeymap is the overlay's `keymap`, as Keymap is the authored file's.
func (c *Config) OverlayKeymap() ([]*kdl.Node, bool) { return children(c.overKeymap) }

func children(n *kdl.Node) ([]*kdl.Node, bool) {
	if n == nil {
		return nil, false
	}
	return n.Children().Nodes, true
}

// Get is the value at key, the overlay's when it sets one, else the authored
// file's, and false when neither does.
func (c *Config) Get(key string) (any, bool) {
	if l, ok := c.over[key]; ok {
		return l.value, true
	}
	l, ok := c.authored[key]
	return l.value, ok
}

// Theme is the configured theme's name, "" for the default.
func (c *Config) Theme() string {
	v, _ := c.Get("theme")
	s, _ := v.(string)
	return s
}

// Overrides is every key the overlay sets, sorted: what kubecom wrote over the
// authored file.
func (c *Config) Overrides() []string {
	keys := make([]string, 0, len(c.over))
	for k := range c.over {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// leaf is one value in a file: the Go value and the node that holds it.
type leaf struct {
	value any
	node  *kdl.Node
}

// readFile parses one config file. A missing file is (nil, empty, nil).
func readFile(path string) (*kdl.Document, map[string]leaf, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, map[string]leaf{}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %v", err)
	}
	doc, err := parse(path, src)
	if err != nil {
		return nil, nil, err
	}
	leaves, err := collect(doc)
	if err != nil {
		return nil, nil, err
	}
	return doc, leaves, nil
}

// parse reads src as a KDL v2 document and checks its version node. The first
// parse error is returned as "<path>:<line>:<col>: <message>".
func parse(path string, src []byte) (*kdl.Document, error) {
	res := kdl.ParseStringWithDiagnostics(string(src), kdl.WithVersion(kdl.Version2), kdl.WithSourceName(path))
	for _, d := range res.Diagnostics {
		if d.Severity == kdl.SeverityError {
			return nil, fmt.Errorf("config: %s: %s", d.Start, d.Message)
		}
	}
	doc := res.Document
	if len(doc.Nodes) == 0 || doc.Nodes[0].Name() != "version" {
		return nil, fmt.Errorf("config: %s: the file must start with `version %d`", path, fileVersion)
	}
	vn := doc.Nodes[0]
	v := vn.Arg(0)
	if len(vn.Arguments()) != 1 || len(vn.Properties()) != 0 || v.Kind() != kdl.Int {
		return nil, fmt.Errorf("config: %s: `version` takes one integer", vn.Location())
	}
	switch n := v.Int(); {
	case n > fileVersion:
		return nil, fmt.Errorf("config: %s: version %d is newer than this kubecom understands (%d)", path, n, fileVersion)
	case n < fileVersion:
		return nil, fmt.Errorf("config: %s: unknown version %d", path, n)
	}
	return doc, nil
}

// collect walks a parsed file (its version node first, already checked) and
// returns its settings by key. A key may be a dotted path through nodes — `theme`
// today, `a.b "x"` or `a { b "x" }` for any later group. A node that is neither a
// value nor a group, a key kubecom does not read and a value of the wrong kind are
// errors naming the node's line and column. A key set twice keeps the last (KDL's
// rightmost-wins).
func collect(doc *kdl.Document) (map[string]leaf, error) {
	out := map[string]leaf{}
	var walk func(nodes []*kdl.Node, prefix string) error
	walk = func(nodes []*kdl.Node, prefix string) error {
		for _, n := range nodes {
			if prefix == "" && n.Name() == "version" {
				return fmt.Errorf("config: %s: `version` is given twice", n.Location())
			}
			if prefix == "" && isDocument(n.Name()) {
				continue // not settings: handed over whole
			}
			key := n.Name()
			if prefix != "" {
				key = prefix + "." + key
			}
			if slices.Contains(strings.Split(key, "."), "") {
				return fmt.Errorf("config: %s: %q is not a key (an empty segment)", n.Location(), key)
			}
			kids := n.Children().Nodes
			args := n.Arguments()
			switch {
			case len(n.Properties()) > 0:
				return fmt.Errorf("config: %s: %s: a setting takes no properties", n.Location(), key)
			case len(kids) > 0 && len(args) > 0:
				return fmt.Errorf("config: %s: %s: a group takes no value", n.Location(), key)
			case len(kids) > 0:
				if err := walk(kids, key); err != nil {
					return err
				}
				continue
			case len(args) == 0:
				if _, block := n.ChildrenInline(); block {
					continue // an empty group
				}
				return fmt.Errorf("config: %s: %s has no value", n.Location(), key)
			case len(args) > 1:
				return fmt.Errorf("config: %s: %s takes one value, not %d", n.Location(), key, len(args))
			}
			s, ok := lookupSetting(key)
			if !ok {
				return fmt.Errorf("config: %s: %s is not a setting (kubecom reads %s; key bindings go in `keymap { }`)",
					n.Location(), key, settingNames())
			}
			if args[0].Kind() != s.Kind {
				return fmt.Errorf("config: %s: %s takes a %s, not a %s", n.Location(), key,
					strings.ToLower(s.Kind.String()), strings.ToLower(args[0].Kind().String()))
			}
			out[key] = leaf{value: fromKDL(args[0]), node: n}
		}
		return nil
	}
	if err := walk(doc.Nodes[1:], ""); err != nil {
		return nil, err
	}
	return out, nil
}

// settingNames lists the settings for an error.
func settingNames() string {
	names := make([]string, len(Settings))
	for i, s := range Settings {
		names[i] = "`" + s.Key + "`"
	}
	return strings.Join(names, ", ")
}

// fromKDL is a KDL value as a setting's Go value: a string, an int, a float64 or
// a bool.
func fromKDL(v kdl.Value) any {
	switch v.Kind() {
	case kdl.String:
		return v.String()
	case kdl.Int:
		return v.Int()
	case kdl.Float:
		return v.Float()
	case kdl.Bool:
		return v.Bool()
	}
	return nil
}

// toKDL is a setting's Go value as a KDL value.
func toKDL(v any) (kdl.Value, error) {
	switch v := v.(type) {
	case string:
		return kdl.NewString(v), nil
	case int:
		return kdl.NewInt(v), nil
	case float64:
		return kdl.NewFloat(v), nil
	case bool:
		return kdl.NewBool(v), nil
	}
	return kdl.Value{}, fmt.Errorf("config: a %T is not a setting's value (a string, a number or a boolean)", v)
}

// atomicWriteFile writes data to path, creating the parent directory (0o700) if
// needed and replacing any existing file atomically: it writes to a temp file in
// the same directory (so os.Rename stays on one filesystem), chmods it 0o600, and
// renames it over path, so a crash mid-write never leaves a truncated file in
// place. Every file kubecom writes — the overlay and the state — goes through it,
// kept private like a kubeconfig (D89/D90).
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: creating %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("config: creating temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename; a successful rename makes
	// this a no-op (the temp name no longer exists).
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: writing %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config: chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: closing %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config: replacing %s: %w", path, err)
	}
	return nil
}
