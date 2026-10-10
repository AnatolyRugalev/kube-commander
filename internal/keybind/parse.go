package keybind

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	kdl "github.com/calico32/kdl-go"
	"github.com/neuroplastio/myelin"
)

// ParseFile reads a whole KDL file holding a `keymap` document — the shape of
// default.kdl and of config.kdl — into one layer's Doc. name is what its lines are
// called in errors. A file with no `keymap` is an empty Doc.
func ParseFile(name string, src []byte, layer string) (myelin.Doc, error) {
	res := kdl.ParseStringWithDiagnostics(string(src), kdl.WithVersion(kdl.Version2), kdl.WithSourceName(name))
	for _, d := range res.Diagnostics {
		if d.Severity == kdl.SeverityError {
			return myelin.Doc{}, fmt.Errorf("%s: %s", d.Start, d.Message)
		}
	}
	for _, n := range res.Document.Nodes {
		if n.Name() == "keymap" {
			return Parse(name, n.Children().Nodes, layer)
		}
	}
	return myelin.Doc{Layer: layer, Source: name}, nil
}

// Parse reads the children of a `keymap` document into myelin's Doc:
//
//	leader "space"
//	group "g" "+goto"
//	bind "g g" "nav.top"
//	bind "g p" "resources.switch" "pods"
//	unbind "ctrl+b"
//	context "logs" { bind "f" "logs.follow" label="follow" }
//
// Every error names its file:line, and any error refuses the whole document: a
// keymap is used whole or not at all. The Doc holds what was read before the first
// error, so a check can still name every mistake in it.
func Parse(file string, nodes []*kdl.Node, layer string) (myelin.Doc, error) {
	p := parser{file: file, doc: myelin.Doc{Layer: layer, Source: file}}
	p.nodes(nodes, "")
	return p.doc, errors.Join(p.errs...)
}

type parser struct {
	file string
	doc  myelin.Doc
	errs []error
}

func (p *parser) fail(n *kdl.Node, format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf("%s: %s", p.source(n), fmt.Sprintf(format, args...)))
}

// source is where n was written, `file:line`: the file its parse named, else the
// one the parser was told.
func (p *parser) source(n *kdl.Node) string {
	file := p.file
	if f := n.Location().Filename; f != "" {
		file = f
	}
	return fmt.Sprintf("%s:%d", file, n.Location().Line)
}

// nodes reads a list of keymap nodes under a context ("": everywhere).
func (p *parser) nodes(nodes []*kdl.Node, when string) {
	for _, n := range nodes {
		switch n.Name() {
		case "leader":
			p.leader(n, when)
		case "base":
			p.base(n, when)
		case "bind":
			p.bind(n, when)
		case "unbind":
			p.unbind(n, when)
		case "group":
			p.group(n, when)
		case "context":
			p.context(n, when)
		default:
			p.fail(n, "%q is not a keymap node (leader, base, bind, unbind, group, context)", n.Name())
		}
	}
}

func (p *parser) leader(n *kdl.Node, when string) {
	s, ok := p.strings(n, 1, 1, nil)
	if !ok {
		return
	}
	if when != "" {
		p.fail(n, "leader cannot be inside a context")
		return
	}
	if _, err := myelin.ParseKey(s[0]); err != nil {
		p.fail(n, "leader: %v", err)
		return
	}
	p.doc.Leader, p.doc.Source = s[0], p.source(n)
}

// base says which defaults the keymap starts from: "kubecom", the defaults (as
// when it is not given), or "none" — every key is then the user's to bind.
func (p *parser) base(n *kdl.Node, when string) {
	s, ok := p.strings(n, 1, 1, nil)
	if !ok {
		return
	}
	switch {
	case when != "":
		p.fail(n, "base cannot be inside a context")
	case s[0] != "kubecom" && s[0] != "none":
		p.fail(n, "base is \"kubecom\" (the defaults) or \"none\", not %q", s[0])
	case s[0] == "none":
		p.doc.Drops, p.doc.Source = []string{Default}, p.source(n)
	default:
		p.doc.Drops = nil
	}
}

// seq checks a key sequence as written, `<leader>` and all, so a bad key is an
// error at its line in the document rather than one at the build.
func (p *parser) seq(n *kdl.Node, s string) bool {
	fields := strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(" \t\n\f\r", r) })
	if len(fields) == 0 {
		p.fail(n, "%s: an empty key sequence", n.Name())
		return false
	}
	for _, f := range fields {
		if f == leaderToken {
			continue
		}
		if _, err := myelin.ParseKey(f); err != nil {
			p.fail(n, "%s: %v", n.Name(), err)
			return false
		}
	}
	return true
}

// leaderToken is how a sequence writes the leader alias, which myelin resolves.
const leaderToken = "<leader>"

func (p *parser) bind(n *kdl.Node, when string) {
	args, ok := p.strings(n, 2, -1, map[string]kdl.ValueKind{"label": kdl.String})
	if !ok || !p.seq(n, args[0]) {
		return
	}
	b := myelin.Bind{Keys: args[0], Action: args[1], When: when, Source: p.source(n)}
	if len(args) > 2 {
		b.Args = args[2:]
	}
	if v := n.Prop("label"); v.Kind() == kdl.String {
		b.Label = v.String()
	}
	p.doc.Binds = append(p.doc.Binds, b)
}

// unbind takes a key sequence away from every lower layer and earlier line where
// its context holds: a binding to nothing, which wins by the same precedence as
// any other, so the key does there what it would do unbound.
func (p *parser) unbind(n *kdl.Node, when string) {
	args, ok := p.strings(n, 1, 1, nil)
	if !ok || !p.seq(n, args[0]) {
		return
	}
	p.doc.Binds = append(p.doc.Binds, myelin.Bind{Keys: args[0], Unbind: true, When: when, Source: p.source(n)})
}

func (p *parser) group(n *kdl.Node, when string) {
	args, ok := p.strings(n, 2, 2, nil)
	if !ok || !p.seq(n, args[0]) {
		return
	}
	p.doc.Groups = append(p.doc.Groups, myelin.Group{Keys: args[0], Label: args[1], When: when, Source: p.source(n)})
}

// context reads a block under an expression, compiled here so a mistake in it is
// an error at its line even with nothing bound under it. A nested block holds
// where both do.
func (p *parser) context(n *kdl.Node, outer string) {
	if len(n.Arguments()) != 1 || n.Arg(0).Kind() != kdl.String || len(n.Properties()) > 0 {
		p.fail(n, "context takes one expression and a block")
		return
	}
	when := n.Arg(0).String()
	if outer != "" {
		when = "(" + outer + ") && (" + when + ")"
	}
	if _, err := Env().Compile(when); err != nil {
		p.fail(n, "%v", err)
		return
	}
	p.nodes(n.Children().Nodes, when)
}

// strings reads a node's arguments as strings (a number is written as its
// digits), at least lo and at most hi of them (hi < 0: no limit), and checks its
// properties against the allowed kinds. A node with children is refused: only
// context has a block.
func (p *parser) strings(n *kdl.Node, lo, hi int, props map[string]kdl.ValueKind) ([]string, bool) {
	if len(n.Children().Nodes) > 0 {
		p.fail(n, "%s takes no block", n.Name())
		return nil, false
	}
	args := n.Arguments()
	if len(args) < lo || (hi >= 0 && len(args) > hi) {
		p.fail(n, "%s takes %s", n.Name(), arity(lo, hi))
		return nil, false
	}
	out := make([]string, len(args))
	for i, a := range args {
		switch a.Kind() {
		case kdl.String:
			out[i] = a.String()
		case kdl.Int:
			out[i] = strconv.Itoa(a.Int())
		default:
			p.fail(n, "%s's argument %d is not a string", n.Name(), i+1)
			return nil, false
		}
	}
	for name, v := range n.Properties() {
		kind, ok := props[name]
		switch {
		case !ok:
			p.fail(n, "%s has no property %q", n.Name(), name)
			return nil, false
		case v.Kind() != kind:
			p.fail(n, "%s's %s is the wrong kind of value", n.Name(), name)
			return nil, false
		}
	}
	return out, true
}

func arity(lo, hi int) string {
	switch {
	case lo == 1 && hi == 1:
		return "one argument"
	case lo == hi:
		return fmt.Sprintf("%d arguments", lo)
	case hi < 0:
		return fmt.Sprintf("at least %d arguments", lo)
	}
	return fmt.Sprintf("%d to %d arguments", lo, hi)
}
