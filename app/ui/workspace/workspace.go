package workspace

import (
	"github.com/AnatolyRugalev/kube-commander/app/client"
	"github.com/AnatolyRugalev/kube-commander/app/focus"
	"github.com/AnatolyRugalev/kube-commander/app/ui/border"
	"github.com/AnatolyRugalev/kube-commander/app/ui/help"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resourceMenu"
	"github.com/AnatolyRugalev/kube-commander/app/ui/resources/namespace"
	"github.com/AnatolyRugalev/kube-commander/app/ui/widgets/popup"
	"github.com/AnatolyRugalev/kube-commander/commander"
	"github.com/gdamore/tcell"
	"github.com/gdamore/tcell/views"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sync"
)

type workspace struct {
	*views.BoxLayout
	focus.Focusable
	view views.View

	container commander.Container
	focus     commander.FocusManager

	popupMu sync.Mutex
	popup   commander.Popup
	menu    *resourceMenu.ResourceMenu
	widget  commander.Widget

	namespace         string
	namespaceResource *commander.Resource

	selectedWidgetId string
}

func (w *workspace) Resize() {
	w.BoxLayout.Resize()
	w.popupMu.Lock()
	if w.popup != nil {
		w.popup.Reposition(w.view)
		w.popup.Resize()
	}
	w.popupMu.Unlock()
}

func (w *workspace) SetView(view views.View) {
	w.view = view
	w.BoxLayout.SetView(view)
}

// Size returns the preferred size in character cells (width, height).
func (w *workspace) Size() (int, int) {
	wi, h := w.BoxLayout.Size()
	return wi, h
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
	w.popupMu.Lock()
	w.popup = popup.NewPopup(w.view, w.Theme(), title, widget, func() {
		w.popupMu.Lock()
		w.popup.OnHide()
		w.popup = nil
		w.popupMu.Unlock()
		w.UpdateScreen()
	})
	w.popup.OnShow()
	w.focus.Focus(w.popup)
	w.popupMu.Unlock()
	w.UpdateScreen()
}

func (w *workspace) UpdateScreen() {
	w.popupMu.Lock()
	if w.popup != nil {
		w.popup.Reposition(w.container.Screen().View())
		w.popup.Resize()
	}
	w.popupMu.Unlock()
	if screen := w.container.Screen(); screen != nil {
		screen.UpdateScreen()
	}
}

func (w *workspace) ScreenHandler() commander.ScreenHandler {
	return w
}

func (w *workspace) UpdateConfig(f commander.ConfigUpdateFunc) error {
	return w.container.ConfigUpdater().UpdateConfig(f)
}

func (w *workspace) ConfigUpdater() commander.ScreenHandler {
	return w
}

func (w *workspace) Status() commander.StatusReporter {
	return w.container.StatusReporter()
}

func (w *workspace) Draw() {
	w.BoxLayout.Draw()
	w.popupMu.Lock()
	if w.popup != nil {
		w.popup.Draw()
	}
	w.popupMu.Unlock()
}

func (w *workspace) HandleEvent(e tcell.Event) bool {
	if w.Status().HandleEvent(e) {
		return true
	}
	w.popupMu.Lock()
	hasPopup := w.popup != nil
	w.popupMu.Unlock()
	if w.focus.HandleEvent(e, !hasPopup) {
		return true
	}
	if hasPopup {
		return false
	}
	switch ev := e.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyCtrlN, tcell.KeyF2:
			namespace.PickNamespace(w, w.namespaceResource, w.SwitchNamespace)
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
	w.namespaceResource = resMap[schema.GroupKind{Kind: "Namespace"}]

	resMenu, err := resourceMenu.NewResourcesMenu(w, w.onMenuSelect, func() {
		namespace.PickNamespace(w, w.namespaceResource, w.SwitchNamespace)
	}, w.container.ResourceProvider())
	if err != nil {
		return err
	}
	w.container.Register(resMenu)

	w.menu = resMenu
	w.menu.OnShow()
	w.widget = help.NewHelpWidget(w.Theme())
	w.BoxLayout.AddWidget(w.menu, 0.0)
	w.BoxLayout.AddWidget(border.NewVerticalLine(w.Theme()), 0.0)
	w.BoxLayout.AddWidget(w.widget, 1.0)
	w.focus = focus.NewFocusManager(w.menu)

	return nil
}

func (w *workspace) Theme() commander.ThemeManager {
	return w.container.Screen().Theme()
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
