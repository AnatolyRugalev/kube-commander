package workspace

import (
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/border"
	errWidget "github.com/AnatolyRugalev/kube-commander/app/ui/err"
	"github.com/AnatolyRugalev/kube-commander/app/ui/help"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resourceMenu"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resources/namespace"
	"github.com/AnatolyRugalev/kube-commander/app/ui/theme"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/listTable"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/popup"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
)

type workspace struct {
	*views.BoxLayout
	focus.Focusable

	container commander.Container
	focus     commander.FocusManager

	popup  commander.Popup
	menu   *resourceMenu.ResourceMenu
	widget commander.Widget

	namespace         string
	namespaceResource *commander.Resource

	selectedWidgetId string
}

func (w *workspace) ResourceProvider() commander.ResourceProvider {
	return w.container.ResourceProvider()
}

func (w *workspace) CommandBuilder() commander.CommandBuilder {
	return w.container.CommandBuilder()
}

func (w *workspace) CommandExecutor() commander.CommandExecutor {
	return w.container.CommandExecutor()
}

func (w *workspace) Client() commander.Client {
	return w.container.Client()
}

func (w *workspace) CurrentNamespace() string {
	return w.namespace

}

func (w *workspace) SwitchNamespace(namespace string) {
	w.namespace = namespace
	w.widget.OnHide()
	w.widget.OnShow()
	w.menu.Render()
	w.UpdateScreen()
}

func NewWorkspace(container commander.Container, namespace string) *workspace {
	return &workspace{
		BoxLayout: views.NewBoxLayout(views.Horizontal),
		container: container,
		namespace: namespace,
	}
}

func (w *workspace) FocusManager() commander.FocusManager {
	return w.focus
}

func (w *workspace) ShowPopup(title string, widget commander.MaxSizeWidget) {
	w.popup = popup.NewPopup(w.container.Screen().View(), title, widget, func() {
		w.popup.OnHide()
		w.popup = nil
		w.UpdateScreen()
	})
	w.popup.OnShow()
	w.focus.Focus(w.popup)
	w.UpdateScreen()
}

func (w *workspace) UpdateScreen() {
	if w.popup != nil {
		w.popup.Reposition(w.container.Screen().View())
		w.popup.Resize()
	}
	if screen := w.container.Screen(); screen != nil {
		screen.UpdateScreen()
	}
}

func (w *workspace) ScreenUpdater() commander.ScreenUpdater {
	return w
}

func (w *workspace) HandleError(err error) {
	w.ShowPopup("Error", errWidget.NewErrorWidget(err))
}

func (w workspace) Draw() {
	w.BoxLayout.Draw()
	if w.popup != nil {
		w.popup.Draw()
	}
}

func (w workspace) Resize() {
	w.BoxLayout.Resize()
	if w.popup != nil {
		w.popup.Reposition(w.container.Screen().View())
		w.popup.Resize()
	}
}

func (w *workspace) HandleEvent(e tcell.Event) bool {
	if w.focus.HandleEvent(e, w.popup == nil) {
		return true
	}
	if w.popup != nil {
		return false
	}
	switch ev := e.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyCtrlN, tcell.KeyF2:
			namespace.PickNamespace(w, w.namespaceResource, w.SwitchNamespace)
		case tcell.KeyCtrlP:
			w.focus.Focus(w.menu)
			w.menu.SelectItem("Pods")
		case tcell.KeyCtrlD:
			w.focus.Focus(w.menu)
			w.menu.SelectItem("Deployments")
		case tcell.KeyCtrlC:
			w.focus.Focus(w.menu)
			w.menu.SelectItem("Configs")
		case tcell.KeyCtrlI:
			w.focus.Focus(w.menu)
			w.menu.SelectItem("Ingresses")
		default:
			if ev.Rune() == '?' {
				help.ShowHelpPopup(w)
				return true
			}
		}
	}
	return false
}

func (w *workspace) Init() error {
	resMap := client.CoreResources()
	w.namespaceResource = resMap["Namespace"]

	resMenu, err := resourceMenu.NewResourcesMenu(w, w.onMenuSelect, func() {
		namespace.PickNamespace(w, w.namespaceResource, w.SwitchNamespace)
	}, w.container.ResourceProvider())
	if err != nil {
		return err
	}

	resMenu.SetStyler(w.styler)
	w.menu = resMenu
	w.menu.OnShow()
	w.widget = help.NewHelpWidget()
	w.BoxLayout.AddWidget(w.menu, 0.0)
	w.BoxLayout.AddWidget(border.NewVerticalLine(theme.Default), 0.0)
	w.BoxLayout.AddWidget(w.widget, 1.0)
	w.focus = focus.NewFocusManager(w.menu)

	return nil
}

func (w *workspace) styler(list commander.ListView, row commander.Row) tcell.Style {
	style := listTable.DefaultStyler(list, row)

	if row != nil && row.Id() == w.selectedWidgetId && (row.Id() != w.menu.SelectedRowId() || !list.IsFocused()) {
		_, bg, _ := theme.Default.Decompose()
		return style.Background(bg).Bold(true).Underline(true)
	}

	return style
}

func (w *workspace) onMenuSelect(_ string, widget commander.Widget) bool {
	if widget != w.widget {
		w.widget.OnHide()
		w.BoxLayout.RemoveWidget(w.widget)
		w.widget = widget
		w.BoxLayout.AddWidget(w.widget, 0.9)
		w.widget.OnShow()
	}
	w.focus.Focus(w.widget)

	return true
}
