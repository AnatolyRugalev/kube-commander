# Land on Pods by default

- Submitted: 2026-08-15
- Priority: normal
- Area: startup view (walked during S02)

kubecom launches into a welcome page (`panememory.go` — the seed menu left, the
welcome pane right). For the on-call job (S02) that page is a wasted first step:
I immediately wanted the Pods table and had to navigate to it.

I want kubecom to **land on the Pods table by default** on launch (all
namespaces, or the current one), rather than the welcome page. The welcome page
can stay reachable, but the default first frame should be the thing an operator
opens the tool to see.
