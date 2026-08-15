# Scrolling past the last log line should restore follow

- Submitted: 2026-08-15
- Priority: high
- Area: logs view (walked during S03)

After scrolling up to read (which pauses follow), getting back to the bottom
does not re-arm follow — only `G` (nav.bottom) does, and the behaviour is pinned
by `TestDownwardScrollDoesNotResumeFollowing`. So scrolling `j` past the last
line just sits at the end, paused, and the stream silently stops being live.

I want: scrolling down **past the last log line** to re-engage follow — the same
re-arm `G` gives, so a reader who scrolled back down to the newest line is
following again without a keypress.
