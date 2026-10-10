<!-- Code generated from internal/tui/keymap (DefaultKeymap); DO NOT EDIT. -->
<!-- Regenerate with: make keys-doc -->

# kubecom default keybindings

These are the built-in **default** bindings, generated from `default.kdl`
(`kubecom keys default` prints it). Rebind anything in your config's
`keymap { }` (see [configuration.md](configuration.md#key-bindings)); run
`kubecom keys` to print what is in force. Vim keys are listed first,
fallbacks second (D10). A key with a context in parentheses acts only there.

## nav

| Action | Keys | Description |
|--------|------|-------------|
| `nav.up` | `k` / `up` | Move up |
| `nav.down` | `j` / `down` | Move down |
| `nav.left` | `h` / `left` | Focus left pane / scroll left |
| `nav.right` | `l` / `right` | Focus right pane / scroll right |
| `nav.drillIn` | `enter` | Open / drill into selection |
| `nav.back` | `esc` | Go back / up a level |
| `nav.top` | `g g` / `home` | Jump to top |
| `nav.bottom` | `G` / `end` | Jump to bottom |
| `nav.halfPageDown` | `ctrl+d` / `pgdn` | Scroll half page down |
| `nav.halfPageUp` | `ctrl+u` / `pgup` | Scroll half page up |
| `nav.pageDown` | `space` | Scroll full page down |
| `nav.pageUp` | `ctrl+b` | Scroll full page up |

## app

| Action | Keys | Description |
|--------|------|-------------|
| `app.filter` | `/` | Filter / search |
| `app.searchNext` | `n` | Next match |
| `app.searchPrev` | `#` | Previous match |
| `app.unhealthy` | `H` | Unhealthy only |
| `app.unhealthyScan` | `U` | Unhealthy anywhere |
| `app.help` | `?` | Toggle help |
| `app.quit` | `q` / `ctrl+c` | Quit |
| `app.palette` | `:` | Command palette |
| `app.update` | — | Update kubecom |

## ns

| Action | Keys | Description |
|--------|------|-------------|
| `ns.switch` | `N` | Switch namespace |

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
| `sort.column` | `S` | Sort: focus the column-header row |
| `sort.clear` | `x` | Clear sort (restore order) |

## menu

| Action | Keys | Description |
|--------|------|-------------|
| `menu.toggle` | `m` | Toggle left menu pane |
| `menu.pin` | `*` | Pin/unpin the kind for this context |

## actions

| Action | Keys | Description |
|--------|------|-------------|
| `actions.menu` | `enter` (`focus == 'table'`) | Act on the selected row |

## res

| Action | Keys | Description |
|--------|------|-------------|
| `res.describe` | `d` | Describe the selected row |
| `res.events` | `E` | List the selected row's events |
| `res.logs` | `L` | View logs for the selected row |
| `res.edit` | `e` | View / edit the selected row's YAML in $EDITOR |
| `res.delete` | `D` | Delete the selected row |
| `res.children` | `P` | Show the selected owner's pods |
| `res.relations` | `g r` | Show what the selected row is related to |

## logs

| Action | Keys | Description |
|--------|------|-------------|
| `logs.follow` | `f` (`logs`) | Toggle log follow (auto-scroll) in the logs viewer |
| `logs.regex` | `ctrl+r` (`logs`) | Toggle regex matching for the logs filter |
| `logs.wrap` | `w` (`logs`) | Toggle line wrapping in the logs viewer |
| `logs.timestamps` | `t` (`logs`) | Toggle timestamps in the logs viewer |
| `logs.previous` | `o` (`logs`) / `ctrl+p` (`logs`) | Toggle logs of the previous (crashed) container instance |
| `logs.select` | `v` (`logs`) / `V` (`logs`) | Select log lines (visual mode) |
| `logs.yank` | `y` (`logs`) | Copy the selected log lines |

## secret

| Action | Keys | Description |
|--------|------|-------------|
| `secret.reveal` | `r` (`viewer.content == 'secret'`) | Reveal / hide secret values in the secret viewer |
| `secret.copy` | `c` (`viewer.content == 'secret'`) | Copy the selected secret value to the clipboard |

## forwards

| Action | Keys | Description |
|--------|------|-------------|
| `forwards.panel` | `F` | Toggle the port-forward panel |
| `forwards.stopAll` | `X` (`forwards`) | Stop all port-forwards (in the panel) |
| `forwards.localPort` | `p` (`picker.id == 'ports'`) | Set the local port for the highlighted port (port picker) |
| `forwards.freeLocal` | `0` (`picker.id == 'ports'`) | Forward the highlighted port on a free local port (port picker) |

## search

| Action | Keys | Description |
|--------|------|-------------|
| `search.cluster` | `ctrl+f` | Search the cluster across kinds |
| `search.allKinds` | `ctrl+a` (`search`) | Toggle searching all kinds (cluster search) |
| `search.allNamespaces` | `ctrl+w` (`search`) | Toggle searching all namespaces (cluster search) |

## confirm

| Action | Keys | Description |
|--------|------|-------------|
| `confirm.accept` | `y` (`confirm`) / `enter` (`confirm`) | Accept the confirm dialog |
| `confirm.decline` | `n` (`confirm`) / `esc` (`confirm`) | Decline the confirm dialog |
