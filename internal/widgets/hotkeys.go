package widgets

import (
	"github.com/AnatolyRugalev/kube-commander/internal/theme"
	ui "github.com/gizak/termui/v3"
	"image"
)

type HasHotKeys interface {
	GetHotKeys() []*HotKey
}

type HotKey struct {
	Name string
	Key  string
}

type HotKeysBar struct {
	*ui.Block
	keys             map[int]*HotKey
	checkedItemStyle ui.Style
	hotKeyStyle      ui.Style
	hotKeyNameStyle  ui.Style
}

func (h *HotKeysBar) SetHotKey(pos int, key, name string) {
	h.keys[pos] = &HotKey{
		Key:  key,
		Name: name,
	}
}

func (h *HotKeysBar) Clear() {
	h.keys = make(map[int]*HotKey)
}

func (h *HotKeysBar) Draw(buf *ui.Buffer) {
	keyCount := 10
	keyAreaSize := 3
	keyAreaSum := keyAreaSize * keyCount
	nameAreaSize := (h.Rectangle.Max.X - keyAreaSum) / keyCount
	x := 0
	blankCellKey := ui.NewCell(' ', h.hotKeyStyle)
	blankCellName := ui.NewCell(' ', h.hotKeyNameStyle)
	for pos := 1; pos <= keyCount; pos++ {
		key, ok := h.keys[pos]

		buf.Fill(blankCellKey, image.Rect(h.Rectangle.Min.X+x, h.Rectangle.Min.Y, h.Rectangle.Min.X+x+keyAreaSize, h.Rectangle.Max.Y))
		if ok {
			cells := ui.ParseStyles(key.Key, h.hotKeyStyle)
			cells = ui.TrimCells(cells, keyAreaSize)
			offset := keyAreaSize - len(cells)
			for _, cx := range ui.BuildCellWithXArray(cells) {
				tx, cell := cx.X, cx.Cell
				buf.SetCell(cell, image.Pt(x+tx+offset, 0).Add(h.Rectangle.Min))
			}
		}
		x += keyAreaSize

		buf.Fill(blankCellName, image.Rect(h.Rectangle.Min.X+x, h.Rectangle.Min.Y, h.Rectangle.Min.X+x+nameAreaSize, h.Rectangle.Max.Y))
		if ok {
			cells := ui.ParseStyles(key.Name, h.hotKeyNameStyle)
			cells = ui.TrimCells(cells, nameAreaSize)
			for _, cx := range ui.BuildCellWithXArray(cells) {
				tx, cell := cx.X, cx.Cell
				buf.SetCell(cell, image.Pt(x+tx, 0).Add(h.Rectangle.Min))
			}
		}
		x += nameAreaSize
	}
}

func NewHotKeysBar() *HotKeysBar {
	h := &HotKeysBar{
		Block:            ui.NewBlock(),
		keys:             make(map[int]*HotKey),
		checkedItemStyle: theme.Theme["checked"].Active,
		hotKeyStyle:      theme.Theme["hotKey"].Active,
		hotKeyNameStyle:  theme.Theme["hotKeyName"].Active,
	}
	h.Border = false
	return h
}
