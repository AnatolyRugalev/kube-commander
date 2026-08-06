package tui

import (
	"github.com/AnatolyRugalev/kube-commander/internal/kube"
	"github.com/AnatolyRugalev/kube-commander/internal/tui/keymap"
)

// This file is the M3 row-action surface (M3-02): the curated set of operations
// that act on the *selected resource row*, the per-kind applicability that decides
// which of them are offered, and the direct-key shortcuts for the common ones. The
// list itself is no longer a menu of its own — since PAL-05d `a` opens the command
// palette's `:action ` stage over rowActionTitles' set, and `:` appends the same set
// to its verbs (D210) — so "the actions menu" below names that list, not a surface. It is deliberately *only* the surface + routing (D107): opening the menu
// (or pressing a direct key) resolves to a rowActionMsg carrying the chosen
// action and the row's identity; each later M3 leg (M3-03…) handles its own
// intent (the YAML viewer, delete confirm, scale prompt, …). Applicability is by
// kind, so all rows of a resource share the same menu.
//
// A rowAction is *not* a keymap.Action — keys live only in the keymap (D11). The
// direct-key actions (describe/logs/edit/delete) each map to a keymap.Action via
// the `key` field so pressing the key and picking the menu entry funnel through the
// same rowActionMsg; the rest are menu-only (no key of their own). The standalone
// read-only YAML view was retired into the edit (View/Edit YAML) action (D135).

// rowActionMsg is the typed intent a chosen row action dispatches: run Action on
// the object Object (a row of Resource). It is originated by the root model — a
// direct key or an actions-menu pick — and consumed by the root model. M3-02
// lands only the surface + routing (D107), so handleRowAction currently surfaces a
// transient "not yet available" toast; each later M3 leg (M3-03…) replaces that
// branch with the real viewer/action for its own intent.
type rowActionMsg struct {
	Action   rowAction
	Resource kube.Resource
	Object   kube.ObjectRef
}

// rowAction identifies one M3 operation on the selected row. It is the stable id
// carried in rowActionMsg and used as the actions-picker value's backing key.
type rowAction string

const (
	rowActionDescribe       rowAction = "describe"
	rowActionLogs           rowAction = "logs"
	rowActionSecret         rowAction = "secret"
	rowActionScale          rowAction = "scale"
	rowActionRolloutRestart rowAction = "rolloutRestart"
	rowActionCordon         rowAction = "cordon"
	rowActionUncordon       rowAction = "uncordon"
	rowActionDrain          rowAction = "drain"
	rowActionSuspend        rowAction = "suspend"
	rowActionResume         rowAction = "resume"
	rowActionPortForward    rowAction = "portForward"
	rowActionExec           rowAction = "exec"
	rowActionEdit           rowAction = "edit"
	rowActionDelete         rowAction = "delete"
	rowActionChildren       rowAction = "children"
)

// rowActionMeta is one row-action's registry entry: the id, the menu title, the
// keymap.Action it is bound to as a direct key ("" = menu-only), the applicability
// predicate against the browsed resource, and whether committing the action opens a
// yes/no confirm before anything happens. The predicate keys on the resource kind
// (and, for the mutating actions, its verbs) so an action only appears for the kinds
// it can act on.
type rowActionMeta struct {
	action   rowAction
	title    string
	key      keymap.Action
	applies  func(kube.Resource) bool
	confirms bool
}

// asksFirst / actsAtOnce spell the rowActionMeta.confirms column, which is the
// registry's declaration of **which verbs ask before they act** (PAL-06/D228): the
// three that call modal.ShowConfirm from their handler in `app.go` (delete,
// rollout-restart, drain) against the twelve that do not. It is a column of the
// registry rather than a list beside it for one reason: `rowActions` is an unkeyed
// composite literal, so adding an action without answering this question does not
// compile. TestRowActionConfirmsMatchesTheHandlers then pins each answer to what the
// handler actually does, so the column cannot drift from the call sites either.
//
// `actsAtOnce` is deliberately *not* "asks nothing": Scale and Port-forward open a
// **prompt** for a value (ShowPrompt), which is a different question — it asks what to
// do, not whether to do it, and it is visible in the label the moment it opens. What
// this column marks is the verb that will stop and ask for permission, which is the
// thing a bare label could not tell you apart from a read-only viewer.
const (
	actsAtOnce = false
	asksFirst  = true
)

// rowActions is the curated M3 action set, in the order the actions menu lists
// them: the read-only viewers and the children drill-down first, then the
// kind-specific operations, then the
// two general object actions (View/Edit YAML, delete) last so a destructive or
// mutating entry never sits under the cursor by default. View/Edit YAML is a viewer
// that can also mutate on save (D135), so it keeps its place in the mutating group.
// Adding an M3 action is a row here plus (if it handles the intent) a case in
// handleRowAction.
var rowActions = []rowActionMeta{
	{rowActionDescribe, "Describe", keymap.ActionDescribe, canGet, actsAtOnce},
	{rowActionLogs, "Logs", keymap.ActionLogs, kindIn("Pod", "Deployment", "ReplicaSet", "StatefulSet", "DaemonSet", "Job", "ReplicationController"), actsAtOnce},
	{rowActionChildren, "Show pods", keymap.ActionChildren, kube.HasChildren, actsAtOnce},
	{rowActionSecret, "Reveal secret", "", kindIn("Secret"), actsAtOnce},
	{rowActionScale, "Scale", "", kindIn("Deployment", "ReplicaSet", "StatefulSet", "ReplicationController"), actsAtOnce},
	{rowActionRolloutRestart, "Rollout restart", "", kindIn("Deployment", "DaemonSet", "StatefulSet"), asksFirst},
	{rowActionCordon, "Cordon", "", kindIn("Node"), actsAtOnce},
	{rowActionUncordon, "Uncordon", "", kindIn("Node"), actsAtOnce},
	{rowActionDrain, "Drain", "", kindIn("Node"), asksFirst},
	{rowActionSuspend, "Suspend", "", kindIn("CronJob"), actsAtOnce},
	{rowActionResume, "Resume", "", kindIn("CronJob"), actsAtOnce},
	{rowActionPortForward, "Port-forward", "", kindIn("Pod", "Service"), actsAtOnce},
	{rowActionExec, "Exec shell", "", kindIn("Pod"), actsAtOnce},
	{rowActionEdit, "View / Edit YAML", keymap.ActionEdit, canGet, actsAtOnce},
	{rowActionDelete, "Delete", keymap.ActionDelete, canDelete, asksFirst},
}

// keyToRowAction maps a direct-key keymap.Action to its rowAction, built once from
// the registry. Only the actions with a non-empty key appear.
var keyToRowAction = func() map[keymap.Action]rowAction {
	m := make(map[keymap.Action]rowAction)
	for _, meta := range rowActions {
		if meta.key != "" {
			m[meta.key] = meta.action
		}
	}
	return m
}()

// rowActionTitles returns the titles of the actions applicable to r, in registry
// order, plus a title→action map so the picked title resolves back. It is the
// source both the actions menu and its resolution use, so the two never drift.
func rowActionTitles(r kube.Resource) ([]string, map[string]rowAction) {
	titles := make([]string, 0, len(rowActions))
	byTitle := make(map[string]rowAction, len(rowActions))
	for _, meta := range rowActions {
		if meta.applies == nil || !meta.applies(r) {
			continue
		}
		titles = append(titles, meta.title)
		byTitle[meta.title] = meta.action
	}
	return titles, byTitle
}

// confirmMarker is appended to the palette label of an action that asks before it acts
// (PAL-06). It is a word rather than the GUI ellipsis convention on purpose: "Delete…"
// is read as "opens a dialog", which would make the *unmarked* Scale and Port-forward
// say something false, since both open a prompt. "(confirm)" claims only what it marks.
const confirmMarker = " (confirm)"

// rowActionLabel is the label the command palette lists an action under: its title,
// plus confirmMarker when committing it opens a confirm rather than acting. It is the
// one place the marker is added, so the `:action ` stage and the `:` verb stage — which
// share paletteRowVerbs (D205 pt 1) — cannot come to label the same action differently.
//
// The marker is part of the **label**, which is the identity a picker.SelectedMsg
// resolves by (D203 pt 3), so the maps that resolve a pick are keyed by the marked
// label too. The title stays bare everywhere else: the toast a dispatched action
// surfaces names the act ("Delete pod web-1"), not the question that preceded it.
func rowActionLabel(a rowAction) string {
	for _, meta := range rowActions {
		if meta.action == a {
			if meta.confirms {
				return meta.title + confirmMarker
			}
			return meta.title
		}
	}
	return string(a)
}

// rowActionApplies reports whether the given action is applicable to r — the guard
// a direct key uses so pressing e.g. `L` (logs) on a non-pod kind is inert, exactly
// as the entry is absent from that kind's actions menu.
func rowActionApplies(a rowAction, r kube.Resource) bool {
	for _, meta := range rowActions {
		if meta.action == a {
			return meta.applies != nil && meta.applies(r)
		}
	}
	return false
}

// rowActionTitle returns an action's human title (its id if somehow unregistered),
// for the transient toast the placeholder handler surfaces until a later leg wires
// the real behaviour.
func rowActionTitle(a rowAction) string {
	for _, meta := range rowActions {
		if meta.action == a {
			return meta.title
		}
	}
	return string(a)
}

// kindIn builds an applicability predicate matching a fixed set of kinds.
func kindIn(kinds ...string) func(kube.Resource) bool {
	set := make(map[string]struct{}, len(kinds))
	for _, k := range kinds {
		set[k] = struct{}{}
	}
	return func(r kube.Resource) bool {
		_, ok := set[r.GVK.Kind]
		return ok
	}
}

// canGet/canDelete gate the verb-generic actions on the resource's own verbs (a
// RESTMapping-derived fact the discovery layer already carries), so an action the
// API server does not allow for the kind is not offered. The unified View/Edit YAML
// action gates on canGet, not update/patch (D135/M3-15c): it is first a *viewer*
// (you need get to render the YAML), and edit is best-effort — a save on a
// read-only resource degrades to a toast on the apply's RBAC error (principle 3),
// exactly as `kubectl edit` lets you open a get-only object and fails only on save.
// A resource with no verbs recorded (the static seed set, or a hermetic test) is
// treated as permissive so the actions stay reachable — the action layer degrades
// on the real RBAC error (principle 3).
func canGet(r kube.Resource) bool    { return hasVerb(r, "get") }
func canDelete(r kube.Resource) bool { return hasVerb(r, "delete") }

func hasVerb(r kube.Resource, verb string) bool {
	if len(r.Verbs) == 0 {
		return true // unknown verb set → permissive; the action degrades on the real error.
	}
	for _, v := range r.Verbs {
		if v == verb {
			return true
		}
	}
	return false
}
