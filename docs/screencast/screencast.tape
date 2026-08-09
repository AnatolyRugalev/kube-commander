# kubecom screencast tape (M5-09) — records docs/screencast.gif with vhs.
#
#     make screencast        # builds ./bin/kubecom, then runs vhs on this file
#
# What the recording needs, none of which a sandbox has (which is why the tape is
# committed but the GIF is recorded by a human — see
# vault/human-tasks/2026-07-30-record-screencast.md):
#
#   * `vhs` on PATH — https://github.com/charmbracelet/vhs
#   * a real cluster as the kubeconfig's current context, with pods running whose
#     names/namespaces match the FILTER below, and at least one of them printing
#     logs (the tour opens a log stream and greps it);
#   * `kubecom` on PATH — `make screencast` puts *this checkout's* binary there, so
#     the GIF always shows the code it was recorded from.
#
# TUNE: the two lines marked `TUNE` below are the only cluster-specific strings in
# the tape. `kube-system` is the default because every cluster has it. Change them
# to whatever reads best on the cluster you record against, re-run
# `make screencast`, and commit the GIF with the tape change.
#
# Every keypress below carries an annotation on the line above it: either
# `# kubecom-action: <id>`, naming the keymap action the press is meant to trigger,
# or `# kubecom-input: <what>` when the keypress is plain text (a shell command, a
# filter query). TestScreencastTapeMatchesTheKeymap (internal/tui/keymap) checks
# every annotated key against the default keymap and fails `make check` on an
# unannotated key — so rebinding an action, or slipping an unchecked keypress into
# the tour, breaks the build instead of quietly turning this tape (and the GIF made
# from it) into a lie about the keys kubecom actually has.

Require kubecom

Output docs/screencast.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 20
Set Framerate 24
Set TypingSpeed 90ms
# A clean prompt to start from: the tour is about kubecom, not about the shell.
# Set up a tmux session for the captions.
Hide
Type@1ms "./docs/screencast/setup.sh" Enter
Sleep 1s

Type@1ms "export PATH=$PWD/bin:$PATH" Enter

# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Welcome to kubecom!'" Enter

# kubecom-input: clear the recording shell
Type@1ms "clear"
# kubecom-input: run it
Enter
Show

# ---------------------------------------------------------------- launch -----
# Bare `kubecom` opens the browse UI on the current context. The welcome page is
# up immediately; discovery fills the resource menu behind it (fast cold start).
# kubecom-input: launch kubecom on the current kubeconfig context
Type "kubecom"
# kubecom-input: run it
Enter
Sleep 1s

# ------------------------------------------------------- browse: open Pods ---
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]The command palette is your central hub for navigation'" Enter
Show

# `:` is the command palette — one place you type what you want to do. The verb and
# its argument are one line in one box (PAL-03a): the tour types the verb, Enters to
# commit it, then types the kind into the same box — `:` → "resource" → Pods.
# kubecom-action: app.palette
Type ":"
Sleep 500ms
# kubecom-input: narrow the palette to the resource switch
Type "resource"
Sleep 500ms
# kubecom-action: nav.drillIn
Enter
Sleep 500ms
# kubecom-input: narrow the palette's resource argument to Pods
Type "pods"
Sleep 500ms
# kubecom-action: nav.drillIn
Enter
Sleep 3s

# ---------------------------------------------------------------- filter -----
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Filter live resources as you type'" Enter
Show

# The table is a live watch, not a snapshot. `/` narrows it as you type; Enter
# keeps the narrowing and hands the keys back to navigation.
# kubecom-action: app.filter
Type "/"
Sleep 500ms
# kubecom-input: filter text
Type "shop"
Sleep 500ms
# kubecom-input: enter
Enter
Sleep 1s
# kubecom-action: nav.down
Down
# kubecom-input: enter
Enter
Sleep 1s

# ------------------------------------------------------------------ logs -----
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Press L to tail logs for the selected pod'" Enter
Show

# `L` gives logs the whole screen and tails the container live.
# kubecom-action: res.logs
Type "L"
Sleep 5s
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Grep logs instantly without interrupting the stream'" Enter
Show

# `/` greps the stream *while it keeps following* — nothing is re-fetched.
# kubecom-action: app.filter
Type "/"
Sleep 500ms
# kubecom-input: TUNE — the log grep query
Type "info"
Sleep 2s
# kubecom-action: nav.back
Escape
# kubecom-action: app.quit
Type "q"
Sleep 1500ms

# -------------------------------------------------------------- describe -----
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Press D to describe the selected resource'" Enter
Show

# `D` describes the selected row in the shared centered viewer; it scrolls like
# any other view.
# kubecom-action: res.describe
Type "D"
Sleep 3s
# kubecom-action: nav.halfPageDown
Ctrl+D
Sleep 2500ms
# kubecom-action: nav.back
Escape
Sleep 1s

# -------------------------------------------------- cluster-wide search -------
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Ctrl+S searches across all resources in the cluster'" Enter
Show

# Ctrl+S searches across kinds at once — Kind, namespace, name — ranked, best
# match first, streaming in as each kind answers.
# kubecom-action: search.cluster
Ctrl+S
Sleep 1500ms
# kubecom-input: TUNE-adjacent — a query broad enough to hit several kinds
Type "dns"
Sleep 4s
# kubecom-action: nav.back
Escape
Sleep 500ms
# kubecom-action: nav.back
Escape
Sleep 1s

# ------------------------------------------------------------------ quit -----
Hide
# kubecom-input: tmux prefix
Ctrl+B
Type@1ms ":set -g status-left '   #[fg=cyan,bold]Thanks for watching'" Enter
Show

# kubecom-action: app.quit
Type "q"
Sleep 2s

Hide
Type@1ms "./docs/screencast/teardown.sh" Enter
Sleep 1s
