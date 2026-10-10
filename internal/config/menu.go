package config

import (
	"fmt"
	"slices"
	"strings"

	kdl "github.com/calico32/kdl-go"
)

// MenuResource is one resource kind in a menu: the kind named by
// group/version/resource so the kube layer can list and watch it generically,
// plus optional display hints. It is the shape of a `menu` document's entries, of
// a pin and of the kind the state file remembers (D193/D240).
type MenuResource struct {
	// Group is the API group ("" for the core group).
	Group string
	// Version is the API version (v1, v1beta1). Required.
	Version string
	// Resource is the plural resource name the API uses (certificates). Required —
	// it is what the dynamic client addresses.
	Resource string
	// Kind is the display/type kind (Certificate). Optional; a menu derives a title
	// from Resource when it is empty.
	Kind string
	// Namespaced marks the resource namespaced; false is cluster-scoped.
	Namespaced bool
	// Section is the menu heading to place the entry under. Optional; the Custom
	// Resources bucket by default.
	Section string
	// Title overrides the row's label. Optional; Kind by default.
	Title string
}

// A resource is written as one node whose argument is the plural and whose
// properties are the rest, in the authored menu and in the state file alike:
//
//	resource "certificates" group="cert-manager.io" version="v1" kind="Certificate" namespaced=#true
var resourceProps = []string{"group", "version", "kind", "namespaced", "section", "title"}

// readResource reads a resource node. extra names further string properties the
// node may carry (a drill owner's namespace and name), returned by name.
func readResource(n *kdl.Node, extra ...string) (MenuResource, map[string]string, error) {
	var r MenuResource
	fail := func(format string, args ...any) (MenuResource, map[string]string, error) {
		return MenuResource{}, nil, fmt.Errorf("config: %s: %s: %s", n.Location(), n.Name(), fmt.Sprintf(format, args...))
	}
	if len(n.Arguments()) != 1 || n.Arg(0).Kind() != kdl.String {
		return fail("takes the plural resource name (%s \"certificates\" …)", n.Name())
	}
	if len(n.Children().Nodes) > 0 {
		return fail("takes no block")
	}
	r.Resource = n.Arg(0).String()
	extras := map[string]string{}
	for name, v := range n.Properties() {
		switch {
		case name == "namespaced":
			if v.Kind() != kdl.Bool {
				return fail("namespaced is #true or #false")
			}
			r.Namespaced = v.Bool()
			continue
		case !slices.Contains(resourceProps, name) && !slices.Contains(extra, name):
			return fail("has no property %q (%s)", name, strings.Join(append(slices.Clone(resourceProps), extra...), ", "))
		case v.Kind() != kdl.String:
			return fail("%s is a string", name)
		}
		switch s := v.String(); name {
		case "group":
			r.Group = s
		case "version":
			r.Version = s
		case "kind":
			r.Kind = s
		case "section":
			r.Section = s
		case "title":
			r.Title = s
		default:
			extras[name] = s
		}
	}
	switch {
	case strings.TrimSpace(r.Resource) == "":
		return fail("the resource name is required")
	case strings.TrimSpace(r.Version) == "":
		return fail("version is required")
	}
	return r, extras, nil
}

// resourceNode writes r as a node named name, empty hints left out.
func resourceNode(name string, r MenuResource) *kdl.Node {
	n := kdl.NewNode(name, kdl.NewString(r.Resource))
	for _, p := range []struct{ key, value string }{{"group", r.Group}, {"version", r.Version}, {"kind", r.Kind}} {
		if p.value != "" {
			n.AddProperty(p.key, kdl.NewString(p.value))
		}
	}
	if r.Namespaced {
		n.AddProperty("namespaced", kdl.NewBool(true))
	}
	for _, p := range []struct{ key, value string }{{"section", r.Section}, {"title", r.Title}} {
		if p.value != "" {
			n.AddProperty(p.key, kdl.NewString(p.value))
		}
	}
	return n
}

// menu is one `menu` document, read: the context it is for ("" with all set: every
// context) and its entries.
type menu struct {
	context   string
	all       bool
	resources []MenuResource
}

// readMenus reads the authored file's `menu` documents: `menu { … }` for every
// context, `menu "<context>" { … }` for one, each a list of `resource` nodes.
func (c *Config) readMenus() ([]menu, error) {
	out := make([]menu, 0, len(c.menus))
	for _, n := range c.menus {
		var m menu
		switch args := n.Arguments(); {
		case len(n.Properties()) > 0 || len(args) > 1 || (len(args) == 1 && args[0].Kind() != kdl.String):
			return nil, fmt.Errorf("config: %s: menu takes a context name, or none for every context, and a block", n.Location())
		case len(args) == 1:
			m.context = args[0].String()
		default:
			m.all = true
		}
		for _, rn := range n.Children().Nodes {
			if rn.Name() != "resource" {
				return nil, fmt.Errorf("config: %s: a menu holds `resource` nodes, not %q", rn.Location(), rn.Name())
			}
			r, _, err := readResource(rn)
			if err != nil {
				return nil, err
			}
			m.resources = append(m.resources, r)
		}
		out = append(out, m)
	}
	return out, nil
}

// Menu is the extra resource kinds the authored file adds to context's menu: the
// entries of every `menu` with no context, then those of `menu "<context>"`, each
// in file order. Load has already read them, so this cannot fail.
func (c *Config) Menu(context string) []MenuResource {
	menus, _ := c.readMenus()
	var all, own []MenuResource
	for _, m := range menus {
		switch {
		case m.all:
			all = append(all, m.resources...)
		case m.context == context:
			own = append(own, m.resources...)
		}
	}
	return append(all, own...)
}

// sameGVR reports whether two menu entries address the same API resource — the
// identity the menu itself dedupes on (menu.AddExtras keys off the GVR), so pins
// agree with it rather than treating two spellings of one kind as two rows.
func sameGVR(a, b MenuResource) bool {
	return a.Group == b.Group && a.Version == b.Version && a.Resource == b.Resource
}
