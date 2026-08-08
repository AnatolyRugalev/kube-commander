# kubecom

A fast, vim-friendly, zero-deploy Kubernetes TUI — *"the kubernetes-dashboard in
your terminal."* Browse and operate any cluster over SSH, in real time, with no
in-cluster deployment and **no `kubectl` binary required**.

> ### 🚧 `v1` is a ground-up rewrite in progress
>
> This branch (`v1`) rebuilds the 2020 codebase from scratch on a modern Go stack
> (**Bubble Tea + client-go**), fixing the old data-race/focus/redraw bug class by
> construction and dropping the hard `kubectl` dependency. The original 2020 code
> lives on [`master`](https://github.com/neuroplastio/kubecom/tree/master).
>
> **Current status:** feature-complete and dogfooded against real clusters —
> everything documented below is in the binary, not planned. What is left is the
> release itself: **no version has been tagged yet**, so you install from a `v1`
> checkout ([Install](#install)), and the Homebrew, AUR and container paths start
> working with that first tag.
>
> The rewrite is driven autonomously and documents itself in **[`vault/`](vault/)**
> (goals, plan, live task board, decision log, per-leg journal); see
> [`CLAUDE.md`](CLAUDE.md) for the operating model.

## Why kubecom

- **Zero deploy.** A single static binary you run locally or over SSH — nothing to
  install in the cluster, no HTTP ingress to expose.
- **Real-time.** Lists are server-side watched and update live; no refresh key.
- **No `kubectl` binary.** Discovery, list/watch, logs, describe, YAML, and the
  action set are all in-process via client-go.
- **Vim-first, fully rebindable.** `hjkl`, `gg`/`G`, `/`, `n`/`N` by default, with
  arrows and classic keys as an equivalent fallback — and every key is
  configurable (no hard-coded keys anywhere).
- **Approachable.** Simpler and more discoverable than k9s by design.

## What kubecom can do

Every key named below is a **default**: all of them are rebindable under `keys:`
in the config, the full list is in [`docs/keybindings.md`](docs/keybindings.md),
and `kubecom keys` prints your effective map. Two keys find the rest: `?` opens
the help overlay, and `:` opens the command palette, which lists every app-wide
verb and every action available on the selected row.

### Browsing and navigating

Run bare `kubecom` and it opens on your current kubeconfig context: the resource
menu on the left, a live table on the right. Lists are **server-side watched**, so
rows appear, change and vanish as the cluster does — there is no refresh key.
`hjkl` (or the arrows) move within a pane and switch focus between them, `gg`/`G`
jump to the ends, `Ctrl+d`/`Ctrl+u` and `Ctrl+f`/`Ctrl+b` page.

Press `R` (`resources.switch`) to change what the table is browsing — it opens the
palette on `:resource `, listing every browsable kind, so this is the pane-free way
to switch and works with the left menu hidden. The rows are Kinds
(`ExternalSecret`), but you can type **whatever you call the kind at the kubectl
prompt**: its plural (`externalsecrets`), any short name the server advertises
(`es`), or its API group (`external-secrets.io`, which narrows to that operator's
kinds when the Kind is the thing you cannot remember). When two API groups define
the same Kind — a `Cluster` per operator is common — both are listed, each with its
group beside it (`Cluster (postgresql.cnpg.io)`), so you can tell them apart.

Press `m` (`menu.toggle`) to hide the left resource-menu pane so the table takes
the full width, and again to bring it back. While the menu is hidden, focus lives
on the table.

Press `Ctrl+n` (`ns.switch`) to change namespace — the same palette line, on
`:namespace `. kubecom reopens on the namespace you last used in that context
([configuration](docs/configuration.md#what-kubecom-remembers)).

`--context` only picks the context to *start* on: press `C` (`ctx.switch`) to
switch to any other context in the kubeconfig without restarting. The list marks
the one you are on; picking it again does nothing, and a context that fails to
connect leaves you exactly where you were. Switching does not rewrite your
kubeconfig's `current-context` — it applies to this session only. A switch also
picks up everything else that is per-context: you land in the namespace that
context was last left in, and its own
[menu file](docs/configuration.md#per-context-menu) is what the resource menu is
built from.

Press `s` (`sort.column`) to sort the table — repeated presses cycle through the
visible columns and both directions — and `S` (`sort.clear`) to restore the
server's own order.

When your cluster runs **metrics-server**, the Pod and Node tables grow **CPU** and
**MEMORY** columns — millicores and mebibytes, the units `kubectl top` prints —
refreshed every ten seconds and sortable like any other column (sorting uses the
measured value, so `100m` ranks above `20m`). An object that has not been scraped
yet shows blank rather than a zero. If your cluster has no metrics API, or it is
installed but down, the columns simply never appear: there is nothing to enable,
and nothing to dismiss.

Press `P` (`res.children`) on a Deployment, ReplicaSet, StatefulSet, DaemonSet,
Job, ReplicationController, Service or Node to switch the table to **that object's
pods**. It is not a snapshot: kubecom reads the owner's selector (or, for a Node,
`spec.nodeName`) and starts an ordinary live watch narrowed by it server-side, so
the child table sorts, filters, colors and takes every row action exactly like any
other — and keeps updating as pods come and go. The status bar names what you are
scoped to (`↳ Deployment/api · app=web`) so a filtered pod list is never mistakable
for the namespace's. `Esc` returns to the owner, with the row you came from still
selected. A Service with no selector, or an owner that has just been deleted,
leaves you where you are with a message rather than showing you every pod in the
namespace.

Mouse capture is **off by default** so your terminal's own click-drag
**select-to-copy** keeps working (names, values, log lines). Press `M`
(`mouse.toggle`) to turn it on — then click a menu item to open it, click a table
row to select it, and scroll the wheel to move through whichever pane the pointer
is over; a `mouse` marker shows on the status bar while it is on. Toggle it back
off to restore native selection. Keyboard navigation is unaffected either way.

### Finding things

Press `/` (`app.filter`) to **search the open table**: typing narrows the rows
live, `Enter` commits the narrowed view so `n`/`N` step through the matches, and
`Esc` clears the filter and brings every row back. **Matched text is highlighted in
the rows that survive**, in every column the filter looks at, so a row that is on
screen for a reason you cannot see is never left unexplained — the marks stay
visible on the row under the cursor too. **Backspace past the start of the query
cancels the search** — one backspace on an empty line closes the field and returns
you to the normal view, so `/` pressed by mistake costs one keystroke. The same
gesture works in the logs view's live grep.

Press `Ctrl+s` (`search.cluster`) to search the whole cluster instead of one table:
type a query and matching objects stream in **across kinds** (Pods, Deployments,
StatefulSets, DaemonSets, Services, ConfigMaps, Secrets, PVCs, Jobs, CronJobs,
Ingresses) in the current namespace, shown as `Kind  namespace/name`. This is a
deliberate, one-shot query — it lists those kinds once per query and never watches
everything.

**`Enter` is the seam between typing and moving.** While you are typing, every key
goes to the query — so `j` types a `j`, and only the arrows move the cursor. Press
`Enter` and the query line dims: the results now take `j`/`k`/`hjkl`, `g`/`G` and
the page keys like any other list, and a second `Enter` jumps to the highlighted
hit — the table switches to that kind with the object selected. `Esc` hands the
keyboard back to the query with your text and results intact, so refining a search
is `Esc`, type, `Enter` again. From the query line `Esc` clears it, and `Esc` on an
empty query closes the search.

Results are **ranked**, best match first — a query that matches the start of a name
beats one that matches after a `-`, which beats one buried mid-word — and hits slot
into place as they stream in, so the best answer rises to the top without waiting
for the sweep to finish. The highlighted row is carried along, so a hit landing
above your cursor never changes what `Enter` opens. Matching is also **fuzzy**,
with nothing to turn on: a query whose letters appear in order but not together
still matches, so `apisrv` finds `api-server` and `kdns` finds `kube-dns`. **The
letters that matched are marked in every result**, cursor row included — for a fuzzy
hit that is the only thing that says why the row is there, since what you typed does
not appear in the name as you typed it. A hit found by a label selector alone marks
nothing, because the selector matched something the name never showed. Fuzzy
matches always rank *below* every name that contains what you typed outright, and
are capped to a small share of the results, so widening the net can only add
results at the bottom — it never pushes an exact match down. The header tracks the
sweep (`searching 4/11 kinds…`) so a slow kind reads as progress rather than a
hang, and says so explicitly when there were more matches than it shows (`first 200
matches — narrow the query`).

A query can also match **labels** instead of (or as well as) the name: type `-l`
followed by a Kubernetes label selector, exactly as you would pass it to `kubectl`.
`-l app=web` finds everything labelled `app=web`, `api -l app=web` narrows that to
objects whose name also contains `api`, and the full selector syntax works
(`-l tier in (fe, be)`, `-l app=web,env!=prod`, `-l !legacy`). The selector is
evaluated by the API server, so it costs nothing extra — it makes the search
*lighter*, not heavier, by filtering rows before they are sent. A selector that
does not parse is reported under the query line and is never sent, so a typo can
never masquerade as an empty cluster.

That curated kind list is the default because it is the cheap one. `Ctrl+a`
(`search.allKinds`) widens the same query to **every kind your cluster exposes** —
CRDs, RBAC, events, nodes, the lot — and `Ctrl+w` (`search.allNamespaces`) widens
it to **every namespace**, without changing the namespace the table itself is
watching. The header says which widen is on (`all kinds`, `all namespaces`), the
query re-runs immediately, and each is off again the next time you open the search:
a widen is per-search, not a mode you can leave on by accident. Widening is
genuinely more work for the API server, so kubecom lists at most eight kinds at a
time rather than hammering the cluster in one burst.

Press `:` (`app.palette`) to open the **command palette** — one place to type what
you want to do. It lists kubecom's app-wide verbs (switch resource, switch
namespace, switch context, search the cluster, toggle the menu or the port-forward
panel, change theme, quit…), each row showing the command's **name** and what it
does — `ns.switch  Switch namespace`. The name is the same id you rebind under
`keys:`, and typing it narrows the list exactly as typing the description does, so
`ns.sw` finds the namespace switcher. Every verb does exactly what its key does, so
the palette is a way in rather than a second set of behaviour.

Verbs that need a value complete it **in the same box**, so the whole thing is one
line of typing: type enough of the verb, press **Space** (or Enter), and the list
becomes that verb's values with the prompt reading `:resource `. So `:res` `␣`
`pods` `Enter` switches the table to Pods, and `:theme ` `mono` `Enter` changes
theme, without a second pop-up appearing. Backspace on an empty value takes you
back to the verb list, and — on a line you typed your way into — so does `Esc`, with
another `Esc` closing the palette. All five value verbs complete this way:
`resource`, `pin`, `theme`, `namespace` and `context`. The last two have to fetch
their values (from the cluster and from your kubeconfig), so their list can appear a
moment after the prompt does; the box says `— loading…` until it lands, and anything
you type meanwhile still narrows it.

A verb's own key is a **shortcut into the same line**: `T` opens the palette already
reading `:theme `, `R` reading `:resource `, `Ctrl+n` reading `:namespace `, `C`
reading `:context ` and `a` reading `:action `, so the key saves you the typing
without taking you to a different box. The namespace row in the left menu opens the
same line. Backspace rewinds to the full verb list, so a key pressed by mistake
still leaves you one keystroke from everything else; `Esc` closes the palette and
puts you back where you were.

**Every pop-up picker filters as you type** — the command palette and the container
picker. There is no filter key to press first: the matching is the same one cluster
search uses, so a typo-free abbreviation finds its value (`ksys` → `kube-system`)
and exact matches always rank above fuzzy ones. `Esc` clears the query, a second
`Esc` closes the picker, and Enter picks the highlighted row. Because the picker is
always taking text, use the **arrow keys** rather than `j`/`k` to move within one
(`j` types a `j`). The one exception is the port picker, where `p` and `0` are
gestures of their own: it keeps the older behaviour of pressing `/` before
filtering.

### Inspecting objects

Press `D` (`res.describe`) for the **describe** output of the selected row — the
same rendering `kubectl describe` produces, in a centered scrollable viewer.

An object's **YAML** is not a viewer at all. Press `e` (`res.edit`) and kubecom
opens the YAML in your own editor, suspending the TUI the way `kubectl edit` does
and restoring it when you quit. Close without changing anything and nothing is sent
— reading the YAML is just an edit you did not make. Save a change and it is
applied through the API, with the server's own validation and a conflict check, so
a stale buffer is refused rather than allowed to clobber someone else's write.
Viewing and editing YAML are deliberately one gesture rather than two keys.

Which editor that is gets settled **once, at startup**, so you find out before you
press `e` rather than at the moment you wanted to change something: `$KUBE_EDITOR`,
else `$EDITOR`, else `$VISUAL`, else the first of `nvim`, `vim`, `nano`, `vi` found
on your `PATH`. Flags work, so `EDITOR="code -w"` is fine, and a variable you set is
always taken as-is — detection only runs when all three are empty. The choice is
written to the log file, and on the rare box with none of the four installed kubecom
still launches and says so instead of failing at `e`. This differs from `kubectl` in
one place on purpose: `$VISUAL` is consulted too, since the Unix convention reserves
it for full-screen editors, which is exactly this case.

Press `L` (`res.logs`) on a Pod — or on a Deployment, ReplicaSet, StatefulSet,
DaemonSet, Job or ReplicationController, which resolves to one of its pods — to open
the **dedicated full-screen logs view**. It opens on the **last 1000 lines** and
tails the container live from there, the equivalent of `kubectl logs -f --tail=1000`,
so opening the logs of a pod that has been up for a week does not replay a week of
output before it reaches *now*. A pod with more than one container asks which one
first — **init** and ephemeral containers are offered too, marked as such, since an
init container's logs are the only thing to read when a pod is stuck in `Init:`. The
header shows the object, the container, and `[following]`/`[paused]`.

The view holds the **most recent 10 000 lines**: a stream left following all afternoon
drops its oldest lines rather than growing without end. Once it has dropped anything the
header says `[trimmed]`, so a `gg` that lands mid-log does not look like the start of one.

In that view, `/` opens a **live grep**: typing narrows the streamed lines *while the
log keeps following*, with a `matched/total` count, and nothing is re-fetched —
`Esc`, or a backspace past the start of the query, clears the filter and the full
stream is still there. Matches are highlighted in the lines they were found in.
`Ctrl+R` (`logs.regex`) switches that grep between plain substring and **regex**
(both case-insensitive; the prompt reads `re/` and the header shows `[re]`), and it
works while you are typing, so a substring you started can become a pattern without
retyping it — a pattern that does not compile yet keeps the last working one and the
header says `invalid regex`.

The view has a **line cursor**, like a table row: `j`/`k` (or the arrow keys) move a
highlighted line, `gg`/`G` jump to the first and last, and the page keys move it by a
screenful. It steps whole **log lines** — a line folded onto several rows by `w` is
one keystroke away, not three — and the page only scrolls once the cursor would
leave it. While the view is following, the cursor rides the newest line; moving it up
pauses following, and a `/` query narrows what it walks to the lines that matched.

On that cursor, `v` (`logs.select`) starts a **visual selection** — vim's key, and vim's
behaviour: `j`/`k` (and `gg`/`G`/the page keys) extend it, `v` again or `Esc` abandons
it, and the header shows `[visual 3]` so you can see how much is held even when the
selection is taller than the screen. `y` (`logs.yank`) **copies** it to your system
clipboard, through the same path `secret.copy` uses, so it works over SSH and inside tmux
(OSC-52); the status bar says how many lines went. With no selection, `y` copies the
cursor's line — and `gg v G y` copies the whole buffer.

What lands on the clipboard is the log, not the screen: no colors, no highlight from your
grep, and a line that `w` folded onto three rows comes back as **one line**, exactly as it
arrived. That is the reason this exists rather than "just select it with the mouse" —
`M` (`mouse.toggle`) plus the terminal's own select-to-copy still works, but it cannot
reach past the visible screen and it copies wrapped rows with the breaks in them. Each
line carries its timestamp exactly when `t` is showing it, so the copy matches what you
were looking at. Selecting pauses tailing for as long as the selection stands — the
stream would otherwise drag one end of it — and the yank hands the stream back if that is
where you were.

Press `f` (`logs.follow`) to pause tailing, or just scroll up (any upward gesture
pauses it so the next line does not yank you back); `f` again resumes and jumps to
the newest line, and so does `G` (`nav.bottom`) — in a live stream "go to the end"
means catch up *and keep up*. Scrolling down by hand does not: a view you paused
stays paused wherever you park it.

A line wider than the screen is clipped, so one log line stays one row: press `w`
(`logs.wrap`) to fold long lines onto continuation rows instead (the header shows
`[wrap]`), or leave it off and use `h`/`l` (or the arrow keys) to scroll sideways —
the header then shows how many columns are hidden to the left, as `[+16]`. Press `t`
(`logs.timestamps`) to put each line's **server timestamp** ahead of its message, the
equivalent of `kubectl logs --timestamps`. It is a display toggle: the timestamps are
already in the buffer, so turning them on or off redraws what is on screen — no
re-fetch, no lost lines, and your grep and scroll position survive. They are off by
default because an RFC3339 timestamp is 30 columns wide; the grep always matches the
*message*, so a query never accidentally matches the clock.

Press `Ctrl+P` (`logs.previous`) to read the container's **previous terminated
instance** instead of the running one — `kubectl logs -p`, and the log that explains
a `CrashLoopBackOff`, since the run that crashed is the one that already ended. It is
a toggle: the header shows `[previous]` while you are on it. Because the two
instances are two different logs, the lines are replaced — but your grep, wrapping
and timestamps are not, so you can ask the same question of both, and the chord works
with the grep field open. A container that has never terminated has no previous log,
and the cluster says so in the status bar. `Esc` clears the grep, then the selection,
then — with neither open — closes the view, as does `q`.

A **Secret**'s contents open through the actions menu (`Reveal secret`), decoded but
**masked**: every value starts hidden, and `r` (`secret.reveal`) is the deliberate
gesture that shows them. `c` (`secret.copy`) copies the value under the cursor to
your system clipboard, so the usual reason to reveal a secret — pasting it somewhere
— does not require putting it on screen at all.

### Acting on objects

Press `a` (`actions.menu`) to see **what you can do to the selected row**: it opens
the palette on `:action `, listing only the actions that apply to that kind. The
object they would act on is named in the title (`Command — Pod default/web-1`), so
you can always see what you are about to act on before you press Enter. The same
entries are in the plain `:` palette, under the app-wide verbs; `a` is the way to
skip past those. An action that stops to ask before it acts is marked `(confirm)`.

The set, by kind:

| Action | Applies to | Key |
|--------|-----------|-----|
| Describe | anything you can `get` | `D` |
| Logs | Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, ReplicationController | `L` |
| Show pods | Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, ReplicationController, Service, Node | `P` |
| Reveal secret | Secret | |
| Scale | Deployment, ReplicaSet, StatefulSet, ReplicationController | |
| Rollout restart | Deployment, DaemonSet, StatefulSet | |
| Cordon / Uncordon / Drain | Node | |
| Suspend / Resume | CronJob | |
| Port-forward | Pod, Service | |
| Exec shell | Pod | |
| View / Edit YAML | anything you can `get` | `e` |
| Delete | anything you can `delete` | `d` |

**Exec shell** drops you into `/bin/sh` inside the container, suspending the TUI the
way the editor does and restoring it when the shell exits — the local terminal goes
raw so `^C` and friends reach the remote PTY, and a resize follows it live. A
multi-container pod asks which container first. When a `kubectl` binary happens to be
on your `PATH`, kubecom suspends into `kubectl exec -it` (pointed at the same cluster,
context and namespace) rather than its own SPDY path, because kubectl is the
battle-tested one; with no kubectl installed the in-process path keeps exec working.

**Port-forward** asks which port — the ports the object actually declares are offered
as a picker, with `p` naming a specific local port and `0` taking any free one, and
free text if the container listens on something it never declared. Forwards run in
the background: `F` (`forwards.panel`) opens a panel listing the live ones from
anywhere in the UI, `Enter` stops the highlighted forward, `X` (`forwards.stopAll`)
stops them all, and quitting kubecom stops them all too.

### Making it yours

- **Keys.** Every action is rebindable in `config.yaml`'s `keys:` map — there are no
  hard-coded keys. See [`docs/keybindings.md`](docs/keybindings.md) for the list and
  the defaults, and run `kubecom keys` to print your effective map.
- **Themes.** `T` (`theme.switch`) opens the palette on `:theme ` and switches
  palette immediately; the pick is written back to `config.yaml` so the next launch
  opens on it. Eleven are built in — Catppuccin (Frappé, Macchiato, Mocha), Dracula,
  gruvbox Dark, Monokai, Nord, Rosé Pine, Solarized Dark and Tokyo Night, all dark.
  See [themes](docs/configuration.md#themes).
- **The resource menu.** Each kubeconfig context can add its own resource types
  (chiefly CRDs the built-in menu doesn't know) from a per-context YAML file. See
  [per-context menu](docs/configuration.md#per-context-menu).
- **Pinned kinds.** A cluster with a lot of CRDs has more kinds than a menu can
  usefully list, so the ones *you* work with can be pinned: press `*` (`menu.pin`) on
  a menu row, or on the table while you're browsing a kind, and kubecom keeps that
  kind in this context's menu whether or not discovery lists it. `*` is a toggle, and
  `:pin ` does the same thing by name — a kind you have never seen in the menu is
  exactly the one you want to pin. See [pinned kinds](docs/configuration.md#pinned-kinds).
- **Where you left off.** kubecom reopens on the last namespace you used *and* the
  last resource kind you had open, per context — including across a context switch,
  so flipping to another cluster and back returns you to what you were reading. The
  kind comes back as a fresh watch, and a kind the cluster doesn't serve is skipped
  silently rather than surfaced as an error. See
  [what kubecom remembers](docs/configuration.md#what-kubecom-remembers).

### When something doesn't work

**A resource that won't list** says why in the table itself, not just in the toast:
open a kind whose LIST the API server refuses and the empty pane carries the reason
(RBAC denied it, credentials rejected, the CRD is gone, an unreachable conversion
webhook…), where the fix is — your machine or the cluster — and the server's own
words underneath. It stays until the list succeeds; kubecom keeps retrying in the
background, and the rows replace it the moment one comes back.

**An expired credential plugin** gets a second look. When the list fails because the
`user.exec` plugin in your kubeconfig failed (an expired AWS SSO session is the usual
one), kubecom runs that plugin once more to capture what it printed — under the UI you
would never see it — and puts its own words in the pane instead of "credentials
rejected". If it recognises the failure and your kubeconfig substantiates the fix, it
then asks whether to run it for you, naming the exact command (`aws sso login
--profile acme-prod`): accept and kubecom suspends into it in this terminal the way
`e` suspends into `$EDITOR`, then retries the request that failed; decline and nothing
runs. It never runs anything you were not asked about, and each offer is good for
exactly one run.

**Hit an error?** Every error kubecom shows you in the status bar is also written to
the log file, in full — the toast clears after five seconds and is clipped to your
terminal width, the log line is neither and carries the underlying cause. Discovery
problems go there too, including any API group that failed to load (the usual reason a
kind is missing from the menu). `tail -f ~/.cache/kubecom/kubecom.log` in a second
terminal while you reproduce, and paste what you see into the bug report.

### From the command line

```bash
kubecom                        # browse the current context, all namespaces
kubecom --context my-cluster --namespace my-ns
kubecom --kubeconfig ~/.kube/other-config -n kube-system

kubecom version                # print build information
kubecom keys                   # print the resolved keymap (defaults + your config)
```

Flags: `--kubeconfig` (path; default `$KUBECONFIG`, else `~/.kube/config`),
`--context` (default the file's current-context), `-n`/`--namespace` (default all
namespaces), `--config` (kubecom config file). A missing or invalid
kubeconfig/context fails with a clear message instead of launching. While the UI runs
it owns the terminal, so all logs (including client-go warnings) go to a file under
your cache dir (`~/.cache/kubecom/kubecom.log` on Linux), never the screen. That holds
for standard error as a whole, not just kubecom's own logging: for the life of the UI
the process's stderr *descriptor* points at that log, so anything a library prints —
an auth plugin refreshing your credentials, for instance — ends up in the file instead
of painted over the panes. The exception is the moments kubecom hands you the terminal
on purpose — the YAML editor on `e`, an exec shell, an accepted re-login — where
stderr is yours again for as long as that program runs, so it can talk to you normally.

## Install

Until the first release is tagged, build from a `v1` checkout — this needs
**Go 1.24+**, and works on Linux and macOS (Windows via WSL2):

```bash
git clone -b v1 https://github.com/neuroplastio/kubecom
cd kube-commander
go install ./cmd/kubecom      # installs kubecom to $(go env GOPATH)/bin
```

Release archives, the container image, Homebrew and the AUR package are all wired
and start working with that first tag. [`docs/install.md`](docs/install.md) has
every path, including why `go install …@v1` cannot work and what changes for a
returning 2020 kube-commander user.

## Configuration

kubecom needs no configuration. When you want some, it reads
`~/.config/kubecom/config.yaml`:

```yaml
theme: monokai
keys:
  nav.down: ["j", "down"]
  nav.up:   ["k", "up"]
```

[`docs/configuration.md`](docs/configuration.md) covers the config file, the
built-in themes, per-context resource menus, pinned kinds, what kubecom remembers
per context, and migrating a config from the 2020 kube-commander.
[`docs/keybindings.md`](docs/keybindings.md) is the generated key reference.

## Contributing

The rewrite is currently driven autonomously against the plan in
[`vault/`](vault/). If you'd like to contribute, please open an issue describing
your intent first so we can align with the milestone plan.

## Special thanks

- [Bubble Tea / Bubbles / Lipgloss](https://github.com/charmbracelet) — the TUI stack
- [client-go](https://github.com/kubernetes/client-go) — in-process Kubernetes access
- [k9s](https://github.com/derailed/k9s) — a contemporary Kubernetes TUI in the same space
