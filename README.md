# kubecom

A fast, vim-friendly, zero-deploy Kubernetes TUI — *"the kubernetes-dashboard in
your terminal."* Browse and operate any cluster over SSH, in real time, with no
in-cluster deployment and **no `kubectl` binary required**.

> ### 🚧 `v1` is a ground-up rewrite in progress
>
> This branch (`v1`) rebuilds the 2020 codebase from scratch on a modern Go stack
> (**Bubble Tea + client-go**), fixing the old data-race/focus/redraw bug class by
> construction and dropping the hard `kubectl` dependency. The original 2020 code
> lives on [`master`](https://github.com/AnatolyRugalev/kube-commander/tree/master).
>
> **Current status:** feature-complete and dogfooded against real clusters. The
> two-pane browse UI, the in-process action set, the dedicated logs view, cluster
> search, the context switcher, metrics columns and themes all work today —
> everything documented below is in the binary, not planned. What is left is the
> release itself: **no version has been tagged yet**, so you install from a `v1`
> checkout (below), and the Homebrew, AUR and container paths start working with
> that first tag.
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
  configurable (no hard-coded keys anywhere). See
  [`docs/keybindings.md`](docs/keybindings.md).
- **Approachable.** Simpler and more discoverable than k9s by design.

## Requirements

- **Linux or macOS** (Windows via WSL2 — native Windows is a non-goal).
- A working **kubeconfig** (kubecom uses your current context by default).
- **Go 1.24+** to build from source (until tagged release binaries ship in M5).

## Install

### From a local checkout (recommended today)

Until a tagged release lands (M5), build from a `v1` checkout — this needs no
version tag and respects the branch:

```bash
git clone -b v1 https://github.com/AnatolyRugalev/kube-commander
cd kube-commander
go install ./cmd/kubecom      # installs kubecom to $(go env GOPATH)/bin
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. Prefer a plain binary in the
current directory? Use `go build -o kubecom ./cmd/kubecom` instead.

> **Why not `go install …@v1`?** In `go install path@v1`, the `@v1` is a *module
> version query*, and `v1` matches Go's semver-prefix form (major version 1), so
> the toolchain looks for a `v1.x.x` release **tag** — which this repo does not yet
> have — and never falls back to the branch named `v1`. A commit SHA is *not*
> semver-parsed, so a pinned remote install does work if you want one:
>
> ```bash
> go install github.com/AnatolyRugalev/kube-commander/cmd/kubecom@<commit-sha>
> ```
>
> A clean `go install …@latest` returns with the first tagged release.

> Release binaries (goreleaser), Homebrew, AUR and the container image below are
> all wired and waiting on the first tagged release (M5); each is documented here
> as it lands. Two notes
> on Homebrew, since the 2020 kube-commander had a tap: it will keep its address
> (`brew tap AnatolyRugalev/kubecom`), and it ships as a **cask**, which Homebrew
> supports on macOS only — on Linux, use the release tarball, the AUR package or
> `go install` above. Until the first tagged release, that tap still serves the
> 2020 formula, so do not install from it expecting kubecom.

### From a release archive

From the first tagged release onward, every release carries `.tar.gz` archives for
`linux`/`darwin` × `amd64`/`arm64` plus a `checksums.txt`. Download the one for your
platform from the [Releases page](https://github.com/AnatolyRugalev/kube-commander/releases),
verify it, and put the binary on your `PATH`:

```bash
sha256sum --check --ignore-missing checksums.txt
tar xzf kubecom_<version>_<os>_<arch>.tar.gz
install -m 0755 kubecom ~/.local/bin/kubecom
```

The archives are the same artifacts the Homebrew cask and the AUR package install,
so this is the no-package-manager path, not a lesser one. Note that macOS binaries
are unsigned: downloaded by hand rather than through Homebrew, they need
`xattr -dr com.apple.quarantine kubecom` before Gatekeeper will run them.

> **Arch Linux users, note the rename.** The AUR package will be **`kubecom-bin`**
> — *not* the 2020 `kube-commander`, whose name this project can no longer publish
> to (`goreleaser` requires the `-bin` suffix on a prebuilt-binary package, and the
> AUR requires the package name to match the repository). `kubecom-bin` does not
> exist until the first tagged release, and `kube-commander` still installs the
> 2020 build, so do not treat one as an upgrade of the other. The two conflict
> deliberately: both own `/usr/bin/kubecom`, so `pacman` will refuse to install
> `kubecom-bin` until `kube-commander` is removed.

### In a container

A convenience path, not the recommended one — kubecom is a local, zero-deploy
tool, and the native binary is always the better install. The image exists to try
kubecom without putting anything on your `PATH`. It is published from the first
tagged release onward:

```bash
docker run --rm -it \
  -v "$HOME/.kube:/root/.kube:ro" \
  ghcr.io/anatolyrugalev/kubecom
```

`-it` is required, not optional: without a TTY the TUI has no terminal to draw
on. The kubeconfig is mounted read-only because kubecom never writes to it —
though note that it also cannot then remember your last-used namespace across
runs, since that state lives beside the config in the (throwaway) container.

Two limits worth knowing before you reach them. The image is distroless — the
binary, a CA bundle and nothing else — so a kubeconfig using an **exec credential
plugin** (`aws`, `gcloud`, `kubelogin`, …) will not authenticate inside it, and
the **Edit** action has no editor to suspend into (there is no `vi` in the image
for the startup detection below to find). Both work fine with the
native binary. Exec-into-a-pod and port-forwarding do work, the latter with the
usual `-p` mapping since the forward binds inside the container.

Tags follow the releases: `ghcr.io/anatolyrugalev/kubecom:v1.2.3` (or `:1.2.3`),
with `:latest` tracking the newest **final** release — never a pre-release.
Images are multi-arch (`linux/amd64`, `linux/arm64`).

## Usage

Run bare `kubecom` to launch the interactive browse UI against your current
kubeconfig context:

```bash
kubecom                        # browse the current context, all namespaces
kubecom --context my-cluster --namespace my-ns
kubecom --kubeconfig ~/.kube/other-config -n kube-system
```

Flags: `--kubeconfig` (path; default `$KUBECONFIG`, else `~/.kube/config`),
`--context` (default the file's current-context), `-n`/`--namespace` (default all
namespaces), `--config` (kubecom config file). A missing or invalid
kubeconfig/context fails with a clear message instead of launching. While the UI
runs it owns the terminal, so all logs (including client-go warnings) go to a file
under your cache dir (`~/.cache/kubecom/kubecom.log` on Linux), never the screen.

**A resource that won't list** says why in the table itself, not just in the
toast: open a kind whose LIST the API server refuses and the empty pane carries
the reason (RBAC denied it, credentials rejected, the CRD is gone, an unreachable
conversion webhook…), where the fix is — your machine or the cluster — and the
server's own words underneath. It stays until the list succeeds; kubecom keeps
retrying in the background, and the rows replace it the moment one comes back.

**Hit an error?** Every error kubecom shows you in the status bar is also written
to that log file, in full — the toast clears after five seconds and is clipped to
your terminal width, the log line is neither and carries the underlying cause.
Discovery problems go there too, including any API group that failed to load (the
usual reason a kind is missing from the menu). `tail -f ~/.cache/kubecom/kubecom.log`
in a second terminal while you reproduce, and paste what you see into the bug report.

`--context` only picks the context to *start* on: press `C` (`ctx.switch`) to
switch to any other context in the kubeconfig without restarting. The picker marks
the one you are on; picking it again does nothing, and a context that fails to
connect leaves you exactly where you were. Switching does not rewrite your
kubeconfig's `current-context` — it applies to this session only. A switch also
picks up everything else that is per-context: you land in the namespace that
context was last left in (below), and its own [menu file](#per-context-menu) is
what the resource menu is built from.

Navigation is keyboard-first (vim keys by default; see
[`docs/keybindings.md`](docs/keybindings.md)). Mouse capture is **off by default**
so your terminal's own click-drag **select-to-copy** keeps working (names, values,
log lines). Press `M` (`mouse.toggle`, rebindable) to turn mouse capture on — then
the mouse is additive: click a menu item to open it, click a table row to select
it, and scroll the wheel to move through whichever pane the pointer is over; a
`mouse` marker shows on the status bar while it is on. Toggle it back off (`M`) to
restore native selection. Keyboard navigation is unaffected either way.

Press `m` (`menu.toggle`, rebindable) to hide the left resource-menu pane — the
table then takes the full width — and press it again to bring the menu back. While
the menu is hidden, focus lives on the table; re-show it to pick a different
resource.

Press `:` (`app.palette`, rebindable) to open the **command palette** — one place to
type what you want to do. It lists kubecom's app-wide verbs (switch resource, switch
namespace, switch context, search the cluster, toggle the menu or the port-forward
panel, change theme, quit…); type to narrow the list and press Enter to run the
highlighted one. Each verb does exactly what its key does, so the palette is a way in
rather than a second set of behaviour — and you never have to remember a key to reach
any of them.

Verbs that need a value complete it **in the same box**, so the whole thing is one line
of typing: type enough of the verb, press **Space** (or Enter), and the list becomes
that verb's values with the prompt reading `:resource `. So `:res` `␣` `pods` `Enter`
switches the table to Pods, and `:theme ` `mono` `Enter` changes theme, without a second
pop-up appearing. Backspace on an empty value takes you back to the verb list, as does
`Esc`; another `Esc` closes the palette. All five value verbs complete this way —
`resource`, `pin`, `theme`, `namespace` and `context`. The last two have to fetch their values
(from the cluster and from your kubeconfig), so their list can appear a moment after the
prompt does; the box says `— loading…` until it lands, and anything you type meanwhile
still narrows it.

A verb's own key is a **shortcut into the same line**: `T` opens the palette already
reading `:theme ` and `R` opens it reading `:resource `, so the key saves you the typing
without taking you to a different box. Backspace or `Esc` from there rewinds to the full
verb list, so a key pressed by mistake still leaves you one keystroke from everything
else. (`Ctrl+n` and `C` still open their own pop-ups for now; they are converted next.)

With a row selected in the table, the palette also lists **what you can do to that
row** — Describe, Logs, View / Edit YAML, Exec shell, Port-forward, Delete and the
rest, exactly the entries the actions menu (`a`) offers for that kind, and only the
ones that apply to it. The object they would act on is named in the palette's own
title (`Command — Pod default/web-1`), so you can always see what you are about to
act on before you press Enter. Picking one does precisely what the actions menu does,
confirmations included: `:del` `Enter` still asks before it deletes.

Press `R` (`resources.switch`, rebindable) to jump straight to the resource list — the
palette opened on `:resource `, listing every browsable resource kind, exactly what the
palette's "Switch resource" verb reaches. Just type to narrow it and press Enter to
switch the table to that kind. This is the pane-free way to change the browsed
resource, so you can work with the left menu hidden.

The rows are Kinds (`ExternalSecret`), but you can type **whatever you call the kind
at the kubectl prompt**: its plural (`externalsecrets`), any short name the server
advertises (`es`), or its API group (`external-secrets.io`, which narrows to that
operator's kinds when the Kind is the thing you cannot remember). And when two API
groups define the same Kind — a `Cluster` per operator is common — both are listed,
each with its group beside it (`Cluster (postgresql.cnpg.io)`), so you can tell them
apart and pick either.

**Every pop-up picker filters as you type** — the command palette (including `T` and
`R`, which open it on `:theme ` and `:resource `), the namespace switcher (`Ctrl+n`),
the context switcher (`C`), the actions menu (`a`) and the container picker. There is
no filter key to press first: the matching is the same one cluster search uses, so a
typo-free abbreviation finds its
value (`ksys` → `kube-system`) and exact matches always rank above fuzzy ones. `Esc`
clears the query, a second `Esc` closes the picker, and Enter picks the highlighted
row. Because the picker is always taking text, use the **arrow keys** rather than
`j`/`k` to move within one (`j` types a `j`). The one exception is the port picker,
where `p` and `0` are gestures of their own: it keeps the older behaviour of pressing
`/` before filtering.

Press `Ctrl+s` (`search.cluster`, rebindable) to search the whole cluster instead of
one table: type a query and matching objects stream in **across kinds** (Pods,
Deployments, StatefulSets, DaemonSets, Services, ConfigMaps, Secrets, PVCs, Jobs,
CronJobs, Ingresses) in the current namespace, shown as `Kind  namespace/name`. Press
Enter on a hit to jump straight to it — the table switches to that kind with the object
selected. Results are **ranked**, best match first — a query that matches the start of a
name beats one that matches after a `-`, which beats one buried mid-word — and hits slot
into place as they stream in, so the best answer rises to the top without waiting for the
sweep to finish. The highlighted row is carried along, so a hit landing above your cursor
never changes what `Enter` opens. `Esc` clears the query, and `Esc` again closes the
search. The header tracks
the sweep (`searching 4/11 kinds…`) so a slow kind reads as progress rather than a hang,
and says so explicitly when there were more matches than it shows (`first 200 matches —
narrow the query`). This is a deliberate, one-shot query (it lists those kinds once per
query, never watches everything); `/` remains the filter that narrows the rows of the
table already open.

Matching is **fuzzy**, with nothing to turn on: a query whose letters appear in order but
not together still matches, so `apisrv` finds `api-server` and `kdns` finds `kube-dns`.
Fuzzy matches are always ranked *below* every name that contains what you typed outright,
and the tighter ones come first, so widening the net can only add results at the bottom of
the list — it never pushes a name you typed exactly further down. They are also capped to a
small share of the results, so a loose query can never crowd out the exact match in a kind
that answered a moment later.

A query can also match **labels** instead of (or as well as) the name: type `-l` followed
by a Kubernetes label selector, exactly as you would pass it to `kubectl`. `-l app=web`
finds everything labelled `app=web`, `api -l app=web` narrows that to objects whose name
also contains `api`, and the full selector syntax works (`-l tier in (fe, be)`,
`-l app=web,env!=prod`, `-l !legacy`). The selector is evaluated by the API server, so it
costs nothing extra — it makes the search *lighter*, not heavier, by filtering rows before
they are sent. A selector that does not parse is reported under the query line and is
never sent, so a typo can never masquerade as an empty cluster.

That curated kind list is the default because it is the cheap one. Press `Ctrl+a`
(`search.allKinds`, rebindable) to widen the same query to **every kind your cluster
exposes** — CRDs, RBAC, events, nodes, the lot. The header adds `all kinds` while the
widen is on, the query re-runs immediately, and the progress line's denominator jumps to
show what you just asked for. It is off again the next time you open the search: the
widen is per-search, not a mode you can leave on by accident. Widening is genuinely more
work for the API server, so kubecom lists at most eight kinds at a time — a wide sweep
takes longer and the progress line shows it, rather than hammering the cluster in one
burst.

The namespace is the other half of the scope, and it widens the same way: `Ctrl+w`
(`search.allNamespaces`, rebindable) searches **every namespace** without changing the
namespace the table itself is watching — when you close the search you are exactly where
you left off. The header swaps the namespace it names for `all namespaces` while the
widen is on. The two widens are independent, so you can search the curated kinds
everywhere, every kind in one namespace, or — pressing both — everything, everywhere.
Like the kind widen, each is off again the next time you open the search.

Press `P` (`res.children`, rebindable) on a Deployment, ReplicaSet, StatefulSet,
DaemonSet, Job, ReplicationController, Service or Node to switch the table to **that
object's pods**. It is not a snapshot: kubecom reads the owner's selector (or, for a
Node, `spec.nodeName`) and starts an ordinary live watch narrowed by it server-side, so
the child table sorts, filters, colors and takes every row action exactly like any other
— and keeps updating as pods come and go. The status bar names what you are scoped to
(`↳ Deployment/api · app=web`) so a filtered pod list is never mistakable for the
namespace's. `Esc` returns to the owner, with the row you came from still selected. A
Service with no selector, or an owner that has just been deleted, leaves you where you
are with a message rather than showing you every pod in the namespace.

When your cluster runs **metrics-server**, the Pod and Node tables grow **CPU** and
**MEMORY** columns — millicores and mebibytes, the units `kubectl top` prints — refreshed
every ten seconds and sortable like any other column (sorting uses the measured value, so
`100m` ranks above `20m`). An object that has not been scraped yet shows blank rather than
a zero. If your cluster has no metrics API, or it is installed but down, the columns
simply never appear: there is nothing to enable, and nothing to dismiss.

Press `L` (`res.logs`, rebindable) on a Pod — or on a Deployment, ReplicaSet,
StatefulSet, DaemonSet, Job or ReplicationController, which resolves to one of its pods
— to open the **dedicated full-screen logs view**. It opens on the **last 1000 lines**
and tails the container live from there (a pod with more than one container asks which
one first — **init** and ephemeral containers are offered too, marked as such, since an
init container's logs are the only thing to read when a pod is stuck in `Init:`) —
the equivalent of `kubectl logs -f --tail=1000`, so opening the logs of a pod that has
been up for a week does not replay a week of output before it reaches *now*. It gives
logs the whole screen rather than a centered box, because throughput is the point. The
header shows the object, the
container, and `[following]`/`[paused]`. Press `/` to open a **live grep**: typing
narrows the streamed lines *while the log keeps following*, with a `matched/total`
count, and nothing is re-fetched — `Esc` clears the filter and the full stream is still
there. Matches are highlighted in the lines they were found in. `Ctrl+R`
(`logs.regex`) switches that grep between plain substring and **regex** (both
case-insensitive; the prompt reads `re/` and the header shows `[re]`), and it works
while you are typing, so a substring you started can become a pattern without retyping
it — a pattern that does not compile yet keeps the last working one and the header says
`invalid regex`. Press `f` (`logs.follow`) to pause tailing, or just scroll up (any upward
gesture pauses it so the next line does not yank you back); `f` again resumes and jumps
to the newest line, and so does `G` (`nav.bottom`) — in a live stream "go to the end"
means catch up *and keep up*, so the jump rejoins the tail. Scrolling down by hand does
not: a view you paused stays paused wherever you park it. A line wider than the screen is
clipped, so one log line stays one
row: press `w` (`logs.wrap`) to fold long lines onto continuation rows instead (the
header shows `[wrap]`), or leave it off and use `h`/`l` (or the arrow keys) to scroll
sideways to the tail — the header then shows how many columns are hidden to the left, as
`[+16]`. Press `t` (`logs.timestamps`) to put each line's **server timestamp** ahead of
its message, the equivalent of `kubectl logs --timestamps`. It is a display toggle:
the timestamps are already in the buffer, so turning them on or off redraws what is on
screen — no re-fetch, no lost lines, and your grep and scroll position survive. They are
off by default because an RFC3339 timestamp is 30 columns wide; the grep always matches
the *message*, so a query never accidentally matches the clock, and `w`/`h`/`l` are
there for the extra width. Press `Ctrl+P` (`logs.previous`) to read the container's
**previous terminated instance** instead of the running one — `kubectl logs -p`, and the
log that explains a `CrashLoopBackOff`, since the run that crashed is the one that already
ended. It is a toggle: the header shows `[previous]` while you are on it and `Ctrl+P`
comes back. Because the two instances are two different logs, the lines are replaced —
but your grep, wrapping and timestamps are not, so you can ask the same question of both,
and the chord works with the grep field open. A container that has never terminated has
no previous log, and the cluster says so in the status bar. `Esc` with no filter open, or
`q`, closes the view. **Describe** output and **secret** content still open in the shared
centered viewer — only logs stream, so only logs get their own screen.

An object's **YAML** is not a viewer at all. Press `e` (`res.edit`) and kubecom opens the
YAML in your own editor, suspending the TUI the way `kubectl edit` does and restoring it
when you quit. Close without changing anything and nothing is sent — reading the YAML is
just an edit you did not make. Save a change and it is applied through the API, with the
server's own validation and a conflict check, so a stale buffer is refused rather than
allowed to clobber someone else's write. Viewing and editing YAML are deliberately one
gesture rather than two keys.

Which editor that is gets settled **once, at startup**, so you find out before you press
`e` rather than at the moment you wanted to change something: `$KUBE_EDITOR`, else
`$EDITOR`, else `$VISUAL`, else the first of `nvim`, `vim`, `nano`, `vi` found on your
`PATH`. Flags work, so `EDITOR="code -w"` is fine, and a variable you set is always taken
as-is — detection only runs when all three are empty. The choice is written to the log
file (`kubecom.log`, below), and on the rare box with none of the four installed kubecom
still launches and says so instead of failing at `e`. This differs from `kubectl` in one
place on purpose: `$VISUAL` is consulted too, since the Unix convention reserves it for
full-screen editors, which is exactly this case.

The other subcommands report information and exit:

```bash
# Print build information
kubecom version

# Print the resolved keymap (defaults merged with your config)
kubecom keys
```

### Configuration

kubecom reads an optional YAML config from
`os.UserConfigDir()/kubecom/config.yaml` (`~/.config/kubecom/config.yaml` on
Linux). It is not required — kubecom runs on sensible defaults. The config's
`keys:` section rebinds any action; see [`docs/keybindings.md`](docs/keybindings.md)
for the action list and defaults, and inspect the effective map any time with
`kubecom keys`.

```yaml
# ~/.config/kubecom/config.yaml
theme: monokai
keys:
  nav.down: ["j", "down"]
  nav.up:   ["k", "up"]
```

#### Theme

`theme:` picks the palette kubecom renders with. Three are built in:

| Name | |
|------|--|
| `default` | dark-friendly, blue accent (used when `theme:` is absent) |
| `monokai` | the classic warm dark palette, cyan accent |
| `solarized-dark` | Solarized's dark variant |

The name is matched ignoring case and surrounding space, but it is never guessed
at: an unknown name launches on the default theme and shows a brief startup notice
listing the ones that exist.

You can also switch theme from inside kubecom: `T` opens the command palette on its
`:theme ` line, listing the built-in palettes (the one you are rendering in is marked
`*`) — the same list `:` `theme` `␣` reaches. The pick repaints immediately,
and the name is written back to `config.yaml` so the next launch opens on it. The
write-back keeps the rest of the file's settings, but it rewrites the file — **YAML
comments and hand-crafted formatting are lost** — so if you keep comments in your
config, set `theme:` by hand instead.

#### Per-context menu

kubecom can add extra resource types (chiefly CRDs the built-in menu doesn't know)
to the browse menu, per kubeconfig context. Each context reads its own file from
`os.UserConfigDir()/kubecom/menus/<context>.yaml` (`~/.config/kubecom/menus/` on
Linux; the context name is sanitized into a safe filename). The file is optional —
a context with no file uses the default menu; a malformed file falls back to the
default menu and shows a brief startup notice rather than failing to launch.

```yaml
# ~/.config/kubecom/menus/my-cluster.yaml
resources:
  - group: cert-manager.io   # omit for the core ("") group
    version: v1
    resource: certificates   # plural, as the API addresses it
    kind: Certificate        # optional; defaults from resource
    namespaced: true         # optional; default false (cluster-scoped)
    section: Custom Resources # optional; default the Custom Resources group
```

An entry whose resource is already in the menu (a seed row or one discovery finds)
is merged, never listed twice.

#### Pinning a kind (`*`)

A cluster with a lot of CRDs has more kinds than a menu can usefully list, so the
ones *you* work with can be pinned: press `*` on a menu row — or on the table while
you're browsing a kind — and kubecom keeps that kind in this context's menu from then
on, whether or not discovery lists it. The status bar confirms what was pinned.

`*` is a **toggle**: press it again on a kind you pinned and the pin goes away, with
no file to edit. The row goes with it — unless discovery lists that kind too, in which
case it stays as an ordinary discovered row, because unpinning means "stop keeping
this for me", not "hide a kind this cluster has".

You can also pin **by name**, without hunting for the row first: the command palette
has a `pin` verb, so `:pin ` `externalsecrets` `Enter` toggles that kind from wherever
you are. Its list is the same one `R` offers, so a kind is found by its plural, its
short name or its API group — which is the point, since a kind you have never seen in
the menu is exactly the one you want to pin. The verb toggles like the key does, and
declines on a hand-written entry for the same reason.

Pins are recorded for you, so they live in the kubecom-managed state file
(`~/.config/kubecom/state/<context>.yaml`, below) rather than in the menu file you
hand-write — pressing `*` never rewrites `menus/<context>.yaml`. Where both name the
same resource, your hand-written entry wins and keeps its title and section; `*` on
such a row says so and changes nothing, since that entry is yours to edit. Only a kind
you pinned with `*` can be unpinned with it — seed rows and discovered rows are not
removable this way. A pinned kind is otherwise an ordinary menu row, listed under
**Custom Resources** unless the menu already places it elsewhere.

#### Remembered namespace

kubecom remembers the last namespace you selected, per kubeconfig context, and
reopens on it next time. The choice is stored in
`os.UserConfigDir()/kubecom/state/<context>.yaml` (`~/.config/kubecom/state/` on
Linux) — a kubecom-managed file, separate from your config and menu files, so
kubecom rewrites it freely without touching anything you hand-edit (it also holds
the kinds you pin, above). Passing
`-n`/`--namespace` overrides the remembered scope for that run (use `-n ""` to
force all namespaces); switching namespace in the UI updates what's remembered.
Switching context (`C`) lands you in *that* context's remembered namespace, and
what you pick afterwards is remembered against it — `-n` names the scope for the
context you launched on, not for every context you visit.

#### Migrating from the 2020 kube-commander

If you have an old `~/.kubecom.yaml` from the original kube-commander, kubecom
migrates it once on first start — when no `config.yaml` exists yet. The old file
held two things, and they migrate differently:

- **Your theme choice is carried over.** If `currentTheme` named a palette kubecom
  still ships, migration writes it to `theme:` in the new `config.yaml` — `monokai`
  stays `monokai`, and the old `solarized` becomes `solarized-dark` (the same
  palette, renamed). The 2020 built-ins with no port yet (`base16`, `paraiso`,
  `twilight`) fall back to the default theme, and the startup notice names the
  themes you *can* pick. Hand-written palettes under `themes:` are not migrated —
  kubecom's themes are built-in, so a custom color set has nowhere to go.
- **The custom resource menu is not.** The old format stored no API
  version/resource, which the per-context menu needs, so migration lists the
  entries it found and you re-add them in a per-context menu file (above).

Migration writes the new `config.yaml` once and shows a brief startup notice with
whatever it could not carry over. Your old `~/.kubecom.yaml` is left untouched. A
malformed legacy file is ignored and never blocks launch.

## Contributing

The rewrite is currently driven autonomously against the plan in
[`vault/`](vault/). If you'd like to contribute, please open an issue describing
your intent first so we can align with the milestone plan.

## Special thanks

- [Bubble Tea / Bubbles / Lipgloss](https://github.com/charmbracelet) — the TUI stack
- [client-go](https://github.com/kubernetes/client-go) — in-process Kubernetes access
- [k9s](https://github.com/derailed/k9s) — a contemporary Kubernetes TUI in the same space
