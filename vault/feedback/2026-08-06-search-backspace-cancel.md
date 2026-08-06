# Backspacing an empty search query should cancel the search

- Submitted: 2026-08-06
- Priority: normal
- Area: search (in-panel `/` search)

Repro: press `/` to start a search, then immediately press `Backspace`
(nothing typed yet, or you've backspaced the query down to empty). Want:
that should cancel the search and return to the normal view — same gesture
you'd expect from "backspace past the start closes the input" in most
search UIs — rather than leaving the search prompt open and empty.
