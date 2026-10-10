package keybind

import "github.com/neuroplastio/myelin"

// Path is what holds the keyboard, as a context expression sees it (D303 pt 3):
// the nodes from the outside in — the cluster, the pane with focus, the table's
// sort mode, a view opened over the pane, an overlay, a text field. A nil or false
// node is not on the path. Steps is the same path as myelin reads it.
//
// In an expression each node is a boolean (is it on the path?) and its attributes
// are qualified names under it: `table`, `table.kind == 'Pod'`, `!field`,
// `picker.id == 'palette'`. An attribute of a node that is not on the path is
// absent, so an expression that reads it does not hold. `focus` is the innermost
// node's name ("" with nothing on the path): `context "table"` holds under any
// overlay opened over the table, `context "focus == 'table'"` only while the table
// itself has the keys.
type Path struct {
	Cluster *Cluster
	// Menu and Table are the pane with focus; at most one is set.
	Menu  bool
	Table *Table
	// Sort is the table's column-header sort mode.
	Sort bool
	// The views opened over the pane; at most one is set.
	Logs      *Logs
	Viewer    *Viewer
	Search    *Search
	Unhealthy bool
	// The overlays; at most one is set.
	Forwards, Help bool
	Picker         *Picker
	Prompt         bool
	Confirm        bool
	// Field is a text field holding the keyboard.
	Field bool
}

// Cluster is the connected cluster: its kubeconfig context and the namespace scope
// ("" is all namespaces).
type Cluster struct{ Context, Namespace string }

// Table is the resource table with focus and the kind it lists.
type Table struct {
	Resource, Group, Kind string
	Namespaced            bool
	// Drilled is a children drill-down; Filtered a committed `/` filter;
	// UnhealthyOnly the `H` narrowing.
	Drilled, Filtered, UnhealthyOnly bool
}

// Logs is the logs view and its toggles.
type Logs struct{ Follow, Wrap, Timestamps, Previous, Selecting bool }

// Viewer is the read-only viewer; Content is what it shows (describe, events,
// secret).
type Viewer struct{ Content string }

// Search is the cluster search and its two widens.
type Search struct{ AllKinds, AllNamespaces bool }

// Picker is the open picker: ID names it (palette, containers, ports, relations),
// Stage is the palette's committed verb, if any.
type Picker struct{ ID, Stage string }

// nodeTypes are the nodes in path order, each with its attributes' types: the
// whole vocabulary a context expression may use besides focus (Schema).
var nodeTypes = []struct {
	name  string
	attrs map[string]myelin.Type
}{
	{"cluster", map[string]myelin.Type{"context": "string", "namespace": "string"}},
	{"menu", nil},
	{"table", map[string]myelin.Type{
		"resource": "string", "group": "string", "kind": "string",
		"namespaced": "bool", "drilled": "bool", "filtered": "bool", "unhealthy_only": "bool",
	}},
	{"sort", nil},
	{"logs", map[string]myelin.Type{
		"follow": "bool", "wrap": "bool", "timestamps": "bool", "previous": "bool", "selecting": "bool",
	}},
	{"viewer", map[string]myelin.Type{"content": "string"}},
	{"search", map[string]myelin.Type{"all_kinds": "bool", "all_namespaces": "bool"}},
	{"unhealthy", nil},
	{"forwards", nil},
	{"help", nil},
	{"picker", map[string]myelin.Type{"id": "string", "stage": "string"}},
	{"prompt", nil},
	{"confirm", nil},
	{"field", nil},
}

// Steps is the path as myelin reads it: its nodes from the outside in, each with
// every attribute its type declares.
func (p Path) Steps() myelin.Path {
	var out myelin.Path
	on := func(name string, set bool) {
		if set {
			out = append(out, myelin.Step{Name: name})
		}
	}
	if c := p.Cluster; c != nil {
		out = append(out, myelin.Step{Name: "cluster", Attrs: map[string]any{"context": c.Context, "namespace": c.Namespace}})
	}
	on("menu", p.Menu)
	if t := p.Table; t != nil {
		out = append(out, myelin.Step{Name: "table", Attrs: map[string]any{
			"resource": t.Resource, "group": t.Group, "kind": t.Kind, "namespaced": t.Namespaced,
			"drilled": t.Drilled, "filtered": t.Filtered, "unhealthy_only": t.UnhealthyOnly,
		}})
	}
	on("sort", p.Sort)
	if l := p.Logs; l != nil {
		out = append(out, myelin.Step{Name: "logs", Attrs: map[string]any{
			"follow": l.Follow, "wrap": l.Wrap, "timestamps": l.Timestamps,
			"previous": l.Previous, "selecting": l.Selecting,
		}})
	}
	if v := p.Viewer; v != nil {
		out = append(out, myelin.Step{Name: "viewer", Attrs: map[string]any{"content": v.Content}})
	}
	if s := p.Search; s != nil {
		out = append(out, myelin.Step{Name: "search", Attrs: map[string]any{"all_kinds": s.AllKinds, "all_namespaces": s.AllNamespaces}})
	}
	on("unhealthy", p.Unhealthy)
	on("forwards", p.Forwards)
	on("help", p.Help)
	if pk := p.Picker; pk != nil {
		out = append(out, myelin.Step{Name: "picker", Attrs: map[string]any{"id": pk.ID, "stage": pk.Stage}})
	}
	on("prompt", p.Prompt)
	on("confirm", p.Confirm)
	on("field", p.Field)
	return out
}

// Innermost is the name of the node that holds the keyboard ("" for an empty
// path): what `focus` is on the whole path.
func (p Path) Innermost() string {
	steps := p.Steps()
	if len(steps) == 0 {
		return ""
	}
	return steps[len(steps)-1].Name
}

// String writes the path from the outside in, each node with the attributes that
// are set: `cluster(context="kind-dev") > table(kind="Pod", namespaced) > field`.
// Two paths that write the same are the same path, so it is also what tells a
// changed path from an unchanged one.
func (p Path) String() string { return myelin.FormatPath(p.Steps()) }
