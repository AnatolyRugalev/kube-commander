package keymap

import (
	"strings"
)

// This file renders the committed keybindings reference (docs/keybindings.md)
// straight from the resolved keymap, so the doc is generated from the registry
// and can never drift from actual bindings (D11) — the same guarantee help.go
// gives the in-app overlay. The drift is guarded by TestKeybindingsDoc, which
// regenerates with `make keys-doc` (`go test ... -update`).

//go:generate go test . -run TestKeybindingsDoc -update

// docHeader is the fixed preamble of the generated doc. The generated-file
// banner keeps humans from hand-editing a file the drift test will overwrite.
const docHeader = `<!-- Code generated from internal/tui/keymap (DefaultKeymap); DO NOT EDIT. -->
<!-- Regenerate with: make keys-doc -->

# kubecom default keybindings

These are the built-in **default** bindings, generated from the action registry
(the single source of truth for keys, D11). Every action is rebindable via the
` + "`keys:`" + ` section of the config; run ` + "`kubecom keys`" + ` to print your
effective map. Vim keys are listed first, fallbacks second (D10).
`

// Markdown renders this keymap as the docs/keybindings.md reference: one table
// per action namespace (the id prefix before the first "."), columns Action /
// Keys / Description, in registry order. Keys and descriptions come straight from
// Keys() and Describe(); nothing here restates a literal key. An action with no
// bound keys renders its Keys cell as an em dash.
func (k *Keymap) Markdown() string {
	var b strings.Builder
	b.WriteString(docHeader)

	var order []string
	groups := map[string][]Action{}
	for _, a := range Actions() {
		g := groupOf(a)
		if _, seen := groups[g]; !seen {
			order = append(order, g)
		}
		groups[g] = append(groups[g], a)
	}

	for _, g := range order {
		b.WriteString("\n## ")
		b.WriteString(g)
		b.WriteString("\n\n| Action | Keys | Description |\n|--------|------|-------------|\n")
		for _, a := range groups[g] {
			b.WriteString("| `")
			b.WriteString(string(a))
			b.WriteString("` | ")
			b.WriteString(docKeys(k.Keys(a)))
			b.WriteString(" | ")
			b.WriteString(a.Describe())
			b.WriteString(" |\n")
		}
	}
	return b.String()
}

// docKeys renders an action's key tokens for a table cell: each token in
// backticks, joined by " / " (vim key first). No bound keys → an em dash.
func docKeys(keys []string) string {
	if len(keys) == 0 {
		return "—"
	}
	quoted := make([]string, len(keys))
	for i, tok := range keys {
		quoted[i] = "`" + tok + "`"
	}
	return strings.Join(quoted, " / ")
}
