package stories

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var storyDir = filepath.Join("..", "..", "stories")

// storySections are the headings every story carries. The shape is not
// bureaucracy: "The situation" is what makes a story walkable by a stranger, "Your
// goal" is what keeps it a goal rather than a script, and "When you're done" is
// what turns a walk into a report. A story missing one of them produces a walk
// nobody can compare to anything.
var storySections = []string{"## The situation", "## Your goal", "## When you're done"}

// keyMentions are the ways a story can leak the answer it is supposed to measure.
//
// This is the executable half of D268 pt 1 and of CONTRIBUTING.md's one hard rule.
// A story that says which key to press measures whether the walker can follow
// instructions; which keys they reach for unprompted *is* the data, so naming one
// destroys the measurement. The patterns are deliberately narrow — they catch the
// instruction forms, not any mention of a word — because a guard that cries wolf
// gets deleted by the third leg that trips it.
var keyMentions = []struct {
	name string
	re   *regexp.Regexp
}{
	// A backticked one- or two-character token: `L`, `gg`, `/`, `:`.
	{"a key in backticks", regexp.MustCompile("`[^`]{1,2}`")},
	// Named keys, backticked or not.
	{"a named key", regexp.MustCompile(`(?i)\b(ctrl|alt|shift)\s*\+|` +
		`\b(escape|esc key|enter key|arrow keys?|spacebar)\b`)},
	// The instruction itself, however the key is spelled.
	{"a press instruction", regexp.MustCompile(`(?i)\b(press|hit|type)\s+(the\s+)?` + "`")},
}

// storyFiles are the stories themselves: every .md directly under stories/, minus
// the directory's own README.
func storyFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(storyDir)
	if err != nil {
		t.Fatalf("read %s: %v", storyDir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" || e.Name() == "README.md" {
			continue
		}
		out = append(out, filepath.Join(storyDir, e.Name()))
	}
	if len(out) == 0 {
		t.Fatalf("no stories in %s", storyDir)
	}
	return out
}

// TestStoriesHaveTheirSections keeps a story walkable by someone who did not write
// it.
func TestStoriesHaveTheirSections(t *testing.T) {
	for _, path := range storyFiles(t) {
		body := read(t, path)
		for _, section := range storySections {
			if !strings.Contains(body, section) {
				t.Errorf("%s has no %q section — see CONTRIBUTING.md for the shape", path, section)
			}
		}
		if !strings.Contains(body, "Fixture: `stories/cluster`") {
			t.Errorf("%s does not say which fixture it is walked against", path)
		}
	}
}

// TestStoriesNameNoKeys is the rule CONTRIBUTING.md calls the one hard rule.
//
// It reads the story's *prose* only — the metadata block at the top names the trace
// file and the fixture in backticks, which are paths, not keys — by skipping lines
// before the first section heading.
func TestStoriesNameNoKeys(t *testing.T) {
	for _, path := range storyFiles(t) {
		for i, line := range prose(t, path) {
			for _, m := range keyMentions {
				if loc := m.re.FindString(line); loc != "" {
					t.Errorf("%s:%d names %s (%q) — a story that supplies the keys measures "+
						"whether the walker can follow instructions, not whether kubecom is "+
						"discoverable (D268 pt 1)", path, i+1, m.name, loc)
				}
			}
		}
	}
}

// TestExactlyOneMainStory keeps the spine unambiguous: the screencast (TAPE-01) and
// docs/usage.md (DOC-04) are both cut against *the* main story, so two of them — or
// none — leaves those legs picking one for themselves, which is how the demo and
// the docs end up describing different products.
func TestExactlyOneMainStory(t *testing.T) {
	var main []string
	for _, path := range storyFiles(t) {
		for _, line := range strings.Split(read(t, path), "\n") {
			l := strings.ToLower(strings.TrimSpace(line))
			if strings.HasPrefix(l, "- main story:") {
				if strings.Contains(l, "yes") {
					main = append(main, path)
				}
				break
			}
		}
	}
	if len(main) != 1 {
		t.Errorf("want exactly one story marked `Main story: yes`, got %d: %v", len(main), main)
	}
}

// prose returns the story's lines from its first section heading onwards, which is
// everything the walker reads as instructions.
func prose(t *testing.T, path string) []string {
	t.Helper()
	lines := strings.Split(read(t, path), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "## ") {
			return lines[i:]
		}
	}
	return nil
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
