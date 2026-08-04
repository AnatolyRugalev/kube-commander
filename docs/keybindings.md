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
| `nav.left` | `h` / `left` | Focus left pane / scroll left |
| `nav.right` | `l` / `right` | Focus right pane / scroll right |
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
| `app.palette` | `:` | Command palette |

## ns

| Action | Keys | Description |
|--------|------|-------------|
| `ns.switch` | `ctrl+n` | Switch namespace |

## resources

| Action | Keys | Description |
|--------|------|-------------|
| `resources.switch` | `R` | Switch resource |

## ctx

| Action | Keys | Description |
|--------|------|-------------|
| `ctx.switch` | `C` | Switch cluster context |

## theme

| Action | Keys | Description |
|--------|------|-------------|
| `theme.switch` | `T` | Switch color theme |

## mouse

| Action | Keys | Description |
|--------|------|-------------|
| `mouse.toggle` | `M` | Toggle mouse capture (off = select text to copy) |

## sort

| Action | Keys | Description |
|--------|------|-------------|
| `sort.column` | `s` | Sort table (cycle column / direction) |
| `sort.clear` | `S` | Clear sort (restore order) |

## menu

| Action | Keys | Description |
|--------|------|-------------|
| `menu.toggle` | `m` | Toggle left menu pane |
| `menu.pin` | `*` | Pin/unpin the kind for this context |

## actions

| Action | Keys | Description |
|--------|------|-------------|
| `actions.menu` | `a` | Act on the selected row |

## res

| Action | Keys | Description |
|--------|------|-------------|
| `res.describe` | `D` | Describe the selected row |
| `res.logs` | `L` | View logs for the selected row |
| `res.edit` | `e` | View / edit the selected row's YAML in $EDITOR |
| `res.delete` | `d` | Delete the selected row |
| `res.children` | `P` | Show the selected owner's pods |

## logs

| Action | Keys | Description |
|--------|------|-------------|
| `logs.follow` | `f` | Toggle log follow (auto-scroll) in the logs viewer |
| `logs.regex` | `ctrl+r` | Toggle regex matching for the logs filter |
| `logs.wrap` | `w` | Toggle line wrapping in the logs viewer |
| `logs.timestamps` | `t` | Toggle timestamps in the logs viewer |
| `logs.previous` | `ctrl+p` | Toggle logs of the previous (crashed) container instance |

## secret

| Action | Keys | Description |
|--------|------|-------------|
| `secret.reveal` | `r` | Reveal / hide secret values in the secret viewer |
| `secret.copy` | `c` | Copy the selected secret value to the clipboard |

## forwards

| Action | Keys | Description |
|--------|------|-------------|
| `forwards.panel` | `F` | Toggle the port-forward panel |
| `forwards.stopAll` | `X` | Stop all port-forwards (in the panel) |
| `forwards.localPort` | `p` | Set the local port for the highlighted port (port picker) |
| `forwards.freeLocal` | `0` | Forward the highlighted port on a free local port (port picker) |

## search

| Action | Keys | Description |
|--------|------|-------------|
| `search.cluster` | `ctrl+s` | Search the cluster across kinds |
| `search.allKinds` | `ctrl+a` | Toggle searching all kinds (cluster search) |
| `search.allNamespaces` | `ctrl+w` | Toggle searching all namespaces (cluster search) |

## confirm

| Action | Keys | Description |
|--------|------|-------------|
| `confirm.accept` | `y` / `enter` | Accept the confirm dialog |
| `confirm.decline` | `n` / `esc` | Decline the confirm dialog |
