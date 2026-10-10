package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kdl "github.com/calico32/kdl-go"
)

// This file is the overlay's write path: every change goes straight to
// config.overlay.kdl (write-through), and nothing here ever opens config.kdl. The
// overlay is kept as its parsed document, so a write changes one node and keeps
// the rest — comments, order, blank lines — as they were.

// Set makes value the overlay's value at key and writes the overlay. key must be
// a setting (Settings) and value of its kind: a string, an int, a float64 or a
// bool. A key already in the overlay is changed where it stands; a new one is
// added under the groups its path names, made where missing.
func (c *Config) Set(key string, value any) error {
	s, ok := lookupSetting(key)
	if !ok {
		return fmt.Errorf("config: %s is not a setting (kubecom reads %s)", key, settingNames())
	}
	v, err := toKDL(value)
	if err != nil {
		return err
	}
	if v.Kind() != s.Kind {
		return fmt.Errorf("config: %s takes a %s, not a %s", key,
			strings.ToLower(s.Kind.String()), strings.ToLower(v.Kind().String()))
	}
	doc := c.overlay
	if doc == nil {
		doc = kdl.NewDocument(kdl.NewNode("version", kdl.NewInt(fileVersion)))
	}
	if l, ok := c.over[key]; ok {
		l.node.SetArg(0, v)
	} else {
		addLeaf(doc, strings.Split(key, "."), v)
	}
	return c.save(doc)
}

// WriteEmptyOverlay writes an overlay of `version 1` alone in dir when there is
// none: what a migration with nothing to carry over leaves, so it runs once.
func WriteEmptyOverlay(dir string) error {
	path := filepath.Join(dir, OverlayName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return atomicWriteFile(path, []byte(fmt.Sprintf("version %d\n", fileVersion)))
}

// addLeaf puts a value node for path into doc, descending through the groups
// already there whose names are the path's next segment and making the rest.
func addLeaf(doc *kdl.Document, path []string, v kdl.Value) {
	for len(path) > 1 {
		var next *kdl.Node
		for _, n := range doc.Nodes {
			if n.Name() == path[0] && len(n.Arguments()) == 0 && len(n.Properties()) == 0 {
				next = n
			}
		}
		if next == nil {
			next = kdl.NewNode(path[0])
			doc.AddNode(next)
		}
		doc, path = next.Children(), path[1:]
	}
	doc.AddNode(kdl.NewNode(path[0], v))
}

// save writes doc as the overlay and makes it the Config's. The edit was made on
// the document in place, so on a failure the overlay is read back from disk: the
// Config then holds what the file does.
func (c *Config) save(doc *kdl.Document) error {
	err := c.write(doc)
	if err != nil {
		if ov, over, rerr := readFile(c.overlayPath); rerr == nil {
			c.overlay, c.over, c.overKeymap = ov, over, lastNode(ov, keymapDocument)
		}
	}
	return err
}

// write formats doc and writes it over the overlay file atomically.
func (c *Config) write(doc *kdl.Document) error {
	over, err := collect(doc)
	if err != nil {
		return err
	}
	text, err := kdl.FormatToString(doc)
	if err != nil {
		return fmt.Errorf("config: %s: %v", c.overlayPath, err)
	}
	if err := atomicWriteFile(c.overlayPath, []byte(text)); err != nil {
		return err
	}
	c.overlay, c.over, c.overKeymap = doc, over, lastNode(doc, keymapDocument)
	return nil
}
