# S03 — Find it in the noise

- Main story: no
- Fixture: `stories/cluster` — run `up.sh` first
- Trace: `~/traces/s03.jsonl`

## The situation

The storefront is up and serving, but somebody in support says checkout
occasionally fails. There is one very chatty workload in the `shop` namespace
writing a steady stream of request lines, most of them uninteresting.

Buried in that stream, a few times a minute, are the lines that are not routine.

## Your goal

Find the failures in the noise, and get to the point where you could say how often
they happen and what they say.

Then keep the stream open and watch it for a moment: new lines are still arriving
while you read. Decide whether that helped you or got in your way.

## When you're done

Tell me:

1. What the failing lines say, and roughly how often they appear.
2. How you narrowed the stream — and whether the narrowing did what you expected
   to the lines arriving after it.
3. Whether you could tell, at any moment, if you were looking at live output or a
   frozen snapshot.
4. If you wanted to keep any of it — copy a line, save the output — say so, and
   whether you found a way.
