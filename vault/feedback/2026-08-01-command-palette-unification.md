# Fold the namespace and resource pickers into one `:` command palette with fuzzy search

- Submitted: 2026-08-01
- Priority: high
- Area: namespace picker, resource picker, command palette, keybindings

Namespace selector and resource type selector should **immediately start filtering by
name**. Ideally they should both launch a command palette with `:namespace` and
`:resource` typed in, and it should activate by `:`.

In the command palette we just make an autocomplete list of all possible actions for now —
namespace, resource, also **contextual actions when a resource is selected**. And use
fuzzy search over it. Hence, when namespace or resource search is done, it automatically
shows resources and namespaces filtered by the prefix.

## Grounding (what already exists)

Not starting from zero — check these before designing:

- **`:` is already bound to `resources.switch`** (`internal/tui/keymap/keymap.go:319`),
  and its help text already calls it "Switch resource (command palette)"
  (`keymap.go:239`). So `:` is the right key and is already free of conflicts; what is
  missing is that it opens a *resource* picker rather than a *general* palette.
- **`ns.switch` is `ctrl+n`** (`keymap.go:318`). Under this proposal it stays as a
  shortcut but becomes sugar for `:namespace ` pre-typed, rather than a separate surface.
- **`ctx.switch` is `C`** (`keymap.go:329`) and `theme.switch` is `T` — worth deciding
  whether those also become palette verbs (`:context`, `:theme`) with the letter keys
  kept as shortcuts. The same question applies to every capital-letter global.
- There is already fuzzy matching in the codebase for cluster search
  (`internal/kube/search.go`) — the palette should reuse that ranking rather than grow a
  second, differently-behaving fuzzy matcher. Note its exact-above-fuzzy band-gap
  invariant (D152 pt 3 / D153); the palette wants the same property, since an exact verb
  match must never rank below a scattered one.

## What I actually want out of it

The unifying idea is that **there is one place you type to make anything happen**, it
autocompletes, and the argument list narrows as you type — rather than N modal pickers
each with their own key and their own filter behaviour. The specific pain that prompted
this is that the namespace and resource pickers do not filter as you type the way the
palette does, so muscle memory does not transfer between them.

"Contextual actions when a resource is selected" means the palette should also offer the
things currently reachable only from the actions menu / hotkeys (logs, edit, describe,
port-forward, delete…) scoped to whatever row is selected — so `:` is discoverable as
"what can I do right now?" without memorising the keymap.

This is almost certainly **larger than one leg** — triage it into slices (palette shell +
fuzzy verb list; `:namespace`/`:resource` argument completion; contextual actions; the
shortcut keys becoming sugar) and land the first one. Please record the keybinding
consequences as a decision, since it supersedes the current one-key-per-picker model.
