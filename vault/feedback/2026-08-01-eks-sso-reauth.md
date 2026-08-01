# Recognise expired AWS SSO credentials on EKS and offer to run `aws sso login`

- Submitted: 2026-08-01
- Priority: normal
- Area: auth, exec credential plugins, error handling

New: when using EKS and AWS SSO auth, can we recognise that we need to reissue SSO
credentials and ask the user if they want to run `aws sso login --profile x` to get them?

## The experience I want

Today an expired SSO session surfaces as some generic auth failure and you are left to
work out that the fix is a shell command in another terminal. Instead: kubecom notices the
exec credential plugin failed *because the SSO session expired specifically*, and offers a
prompt — "SSO session for profile `x` has expired. Run `aws sso login --profile x`? [y/N]"
— runs it, and retries the connection. The profile name comes from the kubeconfig's own
`user.exec` stanza, so it can be quoted back accurately rather than guessed.

## Grounding and cautions

- The mechanism is the **exec credential plugin** in the kubeconfig user entry (`aws eks
  get-token` or `aws-iam-authenticator`). Detection means inspecting the plugin's failure —
  its exit status and stderr — not the apiserver's 401. `aws` writes a recognisable message
  about the SSO session being expired or absent; match on that narrowly, and treat anything
  unmatched as a plain auth error rather than guessing.
- **Only offer what you can substantiate.** If the profile cannot be determined from the
  kubeconfig (`--profile` in `args`, or `AWS_PROFILE` in `env`), fall back to showing the
  generic auth error rather than inventing a command line. A wrong `aws sso login
  --profile <guess>` is worse than no offer.
- **Running it needs the terminal**: `aws sso login` opens a browser and prints a
  verification code, so it wants the same suspend/restore the `$EDITOR` and exec flows
  already use (`internal/tui/edit.go`, and the exec suspend from D125). Reuse that, don't
  invent a second suspend.
- **Keep it opt-in and generic in shape.** kubecom should never run an external auth
  command without asking. And while AWS SSO is the case I hit, the same pattern (plugin
  fails → a known remediation command exists → offer it) applies to `gcloud auth login`
  and Azure; design the seam so those can be added, but only implement the AWS one now
  rather than building a speculative framework.
- Check `vault/goals.md` before starting — if cloud-provider-specific auth handling reads
  as a non-goal, say so in the journal and propose the smallest aligned version (e.g.
  surfacing the plugin's own stderr legibly with the suggested command *shown but not
  run*), which delivers most of the value with none of the provider coupling.

This is a new area with no existing board line, so it needs triage into tasks rather than
a direct implementation.
