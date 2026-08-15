# Namespace switch: the key and the palette verb behave inconsistently

- Submitted: 2026-08-15
- Priority: high
- Area: namespace switching (walked in S01)

`ns.switch` and the `:namespace` palette path feel inconsistent. The trace shows
the namespace switch was the most fiddled-with action of the whole walk — 7 opens
of `ctrl+n` in 2m50s, several within seconds of each other, and a 5s pause before
the first one (the walker stopped to think about how to do it).

I want the two ways to reach a namespace switch reconciled into one consistent
behavior — whichever one the key opens and whichever one `:` + the verb opens
should feel like the same thing.
