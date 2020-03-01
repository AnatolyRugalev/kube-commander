package listTable

import "github.com/gdamore/tcell"

func KeySwitch(event interface{}, keyFunc func(ev *tcell.EventKey) bool) bool {
	switch ev := event.(type) {
	case *tcell.EventKey:
		return keyFunc(ev)
	}
	return false
}
