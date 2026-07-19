<!-- Code generated from internal/tui/keymap (DefaultKeymap); DO NOT EDIT. -->
<!-- Regenerate with: make keys-doc -->

# kubecom default keybindings

These are the built-in **default** bindings, generated from the action registry
(the single source of truth for keys, D11). Every action is rebindable via the
`keys:` section of the config; run `kubecom keys` to print your
effective map. Vim keys are listed first, fallbacks second (D10).

## nav

| Action | Keys | Description |
|--------|------|-------------|
| `nav.up` | `k` / `up` | Move up |
| `nav.down` | `j` / `down` | Move down |
| `nav.left` | `h` / `left` | Focus left pane / collapse |
| `nav.right` | `l` / `right` | Focus right pane / expand |
| `nav.drillIn` | `enter` | Open / drill into selection |
| `nav.back` | `esc` | Go back / up a level |
| `nav.top` | `gg` / `home` | Jump to top |
| `nav.bottom` | `G` / `end` | Jump to bottom |
| `nav.halfPageDown` | `ctrl+d` / `pgdn` | Scroll half page down |
| `nav.halfPageUp` | `ctrl+u` / `pgup` | Scroll half page up |
| `nav.pageDown` | `ctrl+f` | Scroll full page down |
| `nav.pageUp` | `ctrl+b` | Scroll full page up |

## app

| Action | Keys | Description |
|--------|------|-------------|
| `app.filter` | `/` | Filter / search |
| `app.searchNext` | `n` | Next match |
| `app.searchPrev` | `N` | Previous match |
| `app.help` | `?` | Toggle help |
| `app.quit` | `q` / `ctrl+c` | Quit |
