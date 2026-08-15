# S04 — Change something

- Main story: no
- Fixture: `stories/cluster` — run `up.sh` first
- Trace: `~/traces/s04.jsonl`

## The situation

Traffic to the storefront is up and two replicas are not enough. Separately, the
checkout deployment has been running since before a config change landed and
somebody wants it restarted.

This is the first story where you change the cluster rather than read it. The
fixture is disposable, so there is nothing here you can break that `up.sh` will not
put back.

## Your goal

Three things, in whatever order you like:

1. Run more storefront replicas than it has now, and see the result take effect.
2. Restart checkout without deleting it.
3. Look at one object's full definition, change something in it by hand, and put it
   back.

## When you're done

Tell me:

1. Whether you were ever unsure if an action had actually happened.
2. Whether anything asked you to confirm — and whether the confirmation told you
   enough to answer it safely.
3. For the hand edit: where you ended up editing, whether getting back was clean,
   and whether you trusted that it had been applied.
4. Anything you were afraid to press because you could not tell what it would do.
