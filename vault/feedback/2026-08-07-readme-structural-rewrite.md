# README has lost its shine — structural rewrite around capabilities

Priority: normal
Kind: docs

The README has grown by accretion. Every leg that added a capability appended a
paragraph, and nothing ever reorganised. It is 576 lines and no longer reads
like a document anyone designed.

## What is actually wrong

It is structural, not prose quality. The individual paragraphs are fine — several
are genuinely good — but they are in the wrong shape.

- **`## Usage` is lines 139–423.** Nearly 300 lines, half the README, under one
  flat heading. Everything kubecom can do is in there in the order it happened to
  get built, with bolded lead-ins doing the work that headings should be doing.
- **Install comes before you know what the tool is.** Three variants — local
  checkout, release archive, container — occupy lines 45–138, ahead of a single
  concrete thing kubecom does. A reader deciding whether to care has to scroll
  past all of it.
- **The capability surface is invisible.** There are roughly forty actions in the
  keymap — logs with follow/regex/wrap/timestamps/previous-instance, port
  forwarding, secret reveal and copy, cluster-wide search, describe, edit in
  `$EDITOR`, owner→pods navigation, per-context menus, pinning, themes. Someone
  skimming the README cannot see that surface, because it is prose rather than
  structure. A reader should be able to tell in fifteen seconds whether kubecom
  does the thing they need.
- **Reference and narrative are interleaved.** Config file layout, theme YAML,
  and the 2020 migration notes sit inline with the walkthrough. They are
  reference material; they interrupt when read front to back and are hard to find
  when looked up.

## What I want

A structural rewrite organised **around what kubecom can do**, not around the
order things were built.

Roughly: what it is and why, in a screenful. Then the capabilities, grouped so
the surface is legible at a glance — browsing and navigating, inspecting
(describe, YAML, logs, secrets), acting (edit, delete, port-forward), finding
(filter, cluster search, palette), and making it yours (menus, pinning, themes,
keymap). Then install. Then reference, pushed to the back or out to `docs/`.

I am not prescribing the exact sections — you know the capability set better
than a fixed outline would. What matters:

- A reader who has never seen kubecom knows within a screenful whether it is for
  them.
- Every capability is discoverable by skimming headings alone.
- Install is not the first thing, and not three variants deep before the payoff.
- Reference material is separated from narrative.
- It gets **shorter**. If the rewrite is still ~576 lines, something went into
  the README that belonged in `docs/`.

Keep the screencast and the good concrete paragraphs — the failure-mode ones
("A resource that won't list", "An expired credential plugin") are worth
preserving, they just need a home.

## Notes

Treat this as one leg, or split it: an outline leg that restructures headings and
moves reference material out, then a prose pass. Splitting is fine and probably
better — a 576-line rewrite in one commit is not reviewable.

Worth re-reading `docs/keybindings.md` first: it is generated from the action
registry and is the most honest inventory of the capability set we have.
