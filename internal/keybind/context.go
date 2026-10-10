package keybind

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
)

// Path is what holds the keyboard, as a context expression sees it (D303 pt 3):
// the nodes from the outside in — the cluster, the pane with focus, the table's
// sort mode, a view opened over the pane, an overlay, a text field. A nil or false
// node is not on the path.
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

// node is one node of a path as an expression binds it.
type node struct {
	name  string
	attrs map[string]any
}

// focusVar is the variable holding the innermost node's name.
const focusVar = "focus"

// nodeNames are the nodes in path order, each with its attributes' types. It is
// the whole vocabulary a context expression may use besides focus.
var nodeNames = []struct {
	name  string
	attrs map[string]*cel.Type
}{
	{"cluster", map[string]*cel.Type{"context": cel.StringType, "namespace": cel.StringType}},
	{"menu", nil},
	{"table", map[string]*cel.Type{
		"resource": cel.StringType, "group": cel.StringType, "kind": cel.StringType,
		"namespaced": cel.BoolType, "drilled": cel.BoolType, "filtered": cel.BoolType,
		"unhealthy_only": cel.BoolType,
	}},
	{"sort", nil},
	{"logs", map[string]*cel.Type{
		"follow": cel.BoolType, "wrap": cel.BoolType, "timestamps": cel.BoolType,
		"previous": cel.BoolType, "selecting": cel.BoolType,
	}},
	{"viewer", map[string]*cel.Type{"content": cel.StringType}},
	{"search", map[string]*cel.Type{"all_kinds": cel.BoolType, "all_namespaces": cel.BoolType}},
	{"unhealthy", nil},
	{"forwards", nil},
	{"help", nil},
	{"picker", map[string]*cel.Type{"id": cel.StringType, "stage": cel.StringType}},
	{"prompt", nil},
	{"confirm", nil},
	{"field", nil},
}

// nodes is the path's nodes from the outside in.
func (p Path) nodes() []node {
	var out []node
	on := func(name string, set bool) {
		if set {
			out = append(out, node{name: name})
		}
	}
	if c := p.Cluster; c != nil {
		out = append(out, node{"cluster", map[string]any{"context": c.Context, "namespace": c.Namespace}})
	}
	on("menu", p.Menu)
	if t := p.Table; t != nil {
		out = append(out, node{"table", map[string]any{
			"resource": t.Resource, "group": t.Group, "kind": t.Kind, "namespaced": t.Namespaced,
			"drilled": t.Drilled, "filtered": t.Filtered, "unhealthy_only": t.UnhealthyOnly,
		}})
	}
	on("sort", p.Sort)
	if l := p.Logs; l != nil {
		out = append(out, node{"logs", map[string]any{
			"follow": l.Follow, "wrap": l.Wrap, "timestamps": l.Timestamps,
			"previous": l.Previous, "selecting": l.Selecting,
		}})
	}
	if v := p.Viewer; v != nil {
		out = append(out, node{"viewer", map[string]any{"content": v.Content}})
	}
	if s := p.Search; s != nil {
		out = append(out, node{"search", map[string]any{"all_kinds": s.AllKinds, "all_namespaces": s.AllNamespaces}})
	}
	on("unhealthy", p.Unhealthy)
	on("forwards", p.Forwards)
	on("help", p.Help)
	if pk := p.Picker; pk != nil {
		out = append(out, node{"picker", map[string]any{"id": pk.ID, "stage": pk.Stage}})
	}
	on("prompt", p.Prompt)
	on("confirm", p.Confirm)
	on("field", p.Field)
	return out
}

// Innermost is the name of the node that holds the keyboard ("" for an empty
// path): what `focus` is on the whole path.
func (p Path) Innermost() string {
	nodes := p.nodes()
	if len(nodes) == 0 {
		return ""
	}
	return nodes[len(nodes)-1].name
}

// String writes the path from the outside in, each node with the attributes that
// are set: `cluster(context="kind-dev") > table(kind="Pod", namespaced) > field`.
// Two paths that write the same are the same path, so it is also what tells a
// changed path from an unchanged one.
func (p Path) String() string {
	var b strings.Builder
	for i, n := range p.nodes() {
		if i > 0 {
			b.WriteString(" > ")
		}
		b.WriteString(n.name)
		attrs := make([]string, 0, len(n.attrs))
		for a, v := range n.attrs {
			switch v := v.(type) {
			case string:
				if v != "" {
					attrs = append(attrs, fmt.Sprintf("%s=%q", a, v))
				}
			case bool:
				if v {
					attrs = append(attrs, a)
				}
			}
		}
		if len(attrs) > 0 {
			sort.Strings(attrs)
			b.WriteString("(" + strings.Join(attrs, ", ") + ")")
		}
	}
	return b.String()
}

// activations are a path's prefixes as expression inputs: [d] binds the first d
// nodes, every other node false and its attributes absent, and focus the name of
// the d-th.
func (p Path) activations() []map[string]any {
	nodes := p.nodes()
	out := make([]map[string]any, len(nodes)+1)
	for d := range out {
		act := make(map[string]any, 24)
		for _, n := range nodeNames {
			act[n.name] = false
		}
		act[focusVar] = ""
		for _, n := range nodes[:d] {
			act[n.name] = true
			act[focusVar] = n.name
			for a, v := range n.attrs {
				act[n.name+"."+a] = v
			}
		}
		out[d] = act
	}
	return out
}

// fieldDepth is the depth at which the field joins the path — the depth a binding
// must reach to take a text key from it (D303 pt 5) — or -1 with no field.
func (p Path) fieldDepth() int {
	if !p.Field {
		return -1
	}
	return len(p.nodes())
}

// Context is a compiled context expression: a CEL boolean over the path.
type Context struct {
	src string
	prg cel.Program
}

// String is the expression as written; "" for none.
func (c *Context) String() string {
	if c == nil {
		return ""
	}
	return c.src
}

// celEnv is the one environment every expression is compiled in, built on first
// use.
var celEnv = sync.OnceValues(func() (*cel.Env, error) {
	opts := []cel.EnvOption{cel.Variable(focusVar, cel.StringType)}
	for _, n := range nodeNames {
		opts = append(opts, cel.Variable(n.name, cel.BoolType))
		for a, t := range n.attrs {
			opts = append(opts, cel.Variable(n.name+"."+a, t))
		}
	}
	return cel.NewEnv(opts...)
})

// CompileContext compiles a context expression. A syntax error, a name the path
// does not have, a type error and an expression that is not a boolean are all
// errors here, so a keymap that loads never fails on a keystroke.
func CompileContext(src string) (*Context, error) {
	env, err := celEnv()
	if err != nil {
		return nil, err
	}
	ast, iss := env.Compile(src)
	if iss.Err() != nil {
		var msgs []string
		unknown := false
		for _, e := range iss.Errors() {
			msgs = append(msgs, strings.TrimSuffix(e.Message, " (in container '')"))
			unknown = unknown || strings.HasPrefix(e.Message, "undeclared reference")
		}
		err := fmt.Errorf("context %q: %s", src, strings.Join(msgs, "; "))
		if unknown {
			err = fmt.Errorf("%w (a context reads %s)", err, vocabulary())
		}
		return nil, err
	}
	if !ast.OutputType().IsExactType(cel.BoolType) {
		return nil, fmt.Errorf("context %q is a %s, not a boolean", src, ast.OutputType())
	}
	prg, err := env.Program(ast, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		return nil, fmt.Errorf("context %q: %w", src, err)
	}
	return &Context{src: src, prg: prg}, nil
}

// vocabulary is the names an expression may read, for an error that met another.
func vocabulary() string {
	names := []string{focusVar}
	for _, n := range nodeNames {
		names = append(names, n.name)
	}
	return strings.Join(names, ", ")
}

// Holds reports whether the expression holds on the whole path; a nil context
// holds everywhere.
func (c *Context) Holds(p Path) bool {
	if c == nil {
		return true
	}
	acts := p.activations()
	return c.holds(acts[len(acts)-1])
}

// holds evaluates the expression over one activation. An evaluation error — an
// attribute of a node not on the path — does not hold.
func (c *Context) holds(act map[string]any) bool {
	out, _, err := c.prg.Eval(act)
	return err == nil && out == types.True
}

// depth is how deep into the path the expression reaches: the shortest prefix it
// already holds on, given that it holds on the whole path; -1 when it does not
// hold. A nil context is everywhere, at depth 0, and so is an expression that
// holds with nothing on the path (`!field`): saying what is *not* there adds no
// depth.
func (c *Context) depth(acts []map[string]any) int {
	if c == nil {
		return 0
	}
	if !c.holds(acts[len(acts)-1]) {
		return -1
	}
	for d := 0; d < len(acts)-1; d++ {
		if c.holds(acts[d]) {
			return d
		}
	}
	return len(acts) - 1
}
