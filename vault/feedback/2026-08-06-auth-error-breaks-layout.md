# Context auth errors break the layout

- Submitted: 2026-08-06
- Priority: high
- Area: auth / context switching, layout

When a context fails to authenticate (e.g. an expired/failing exec credential
plugin, bad SSO session), the resulting error is breaking the TUI layout —
not just showing an error message cleanly, but visibly distorting the
surrounding UI. Reproduce with a context whose exec credential plugin
fails (see AUTH-01/D-whatever classifies these failures already) and watch
what happens to the screen around the error.

Want: the auth-error state should render like any other error surface —
contained, not corrupting layout — regardless of how long the message is or
which panel is focused when it happens.
