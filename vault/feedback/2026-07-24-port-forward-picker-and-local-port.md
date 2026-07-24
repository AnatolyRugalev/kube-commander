# Port-forward: add a port picker + handle "local port in use" gracefully

- Submitted: 2026-07-24
- Priority: high
- Area: port-forward (M3-13)

Two problems found dogfooding port-forward:

**1. No port picker.** Starting a forward prompts for ports as free text. Instead,
offer a **picker of the pod's declared ports** (containerPorts / the Service's
ports) so you pick a known target port, and let the **local port** be chosen too
(default = same as remote, editable).

**2. Bind failure surfaces as a raw error and the forward fails:**

```
port-forward Pod data/cache-0: unable to listen on any of the requested ports: [{6379 6379}]
```

Root cause: the **local** port 6379 is already in use (that's `client-go`'s
"can't bind the local listener" error; `{6379 6379}` = {local, remote}, and
`cache-0`:6379 is Redis, which is likely also running locally). Fixes:
- Let the user **choose a different local port** (the picker above), and/or support
  **auto-assign a free local port** (local port `0` → the OS picks; client-go
  reports the bound port — show it in the panel/status bar).
- On bind failure, a **clear status-bar toast**: e.g. "local port 6379 already in
  use — pick another (or 0 for auto)", not the raw listener error. Degrade, don't
  just fail silently to nothing.

Goal: forwarding a Redis pod when local 6379 is taken should be a one-keystroke
"use a free port" rather than a dead-end error.
