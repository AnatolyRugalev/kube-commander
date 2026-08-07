package logsview

import (
	"strconv"
	"testing"

	"github.com/neuroplastio/kubecom/internal/tui/styles"
)

// These measure what it costs to stream n lines into the view — the cost the throughput
// human-task raised and LOGS-05b answers. They are the part of that question the sandbox
// can actually answer: not "does it feel fast against a real cluster", but how the cost of
// a line grows with the number of lines already held.
//
// BenchmarkStreamLines is the pathological shape: one line, one viewport sync. It stays
// superlinear on purpose — the viewport re-measures every line it holds to find the
// longest, and it has no append API — which is exactly why the pump batches (LOGS-05b).
// BenchmarkStreamLinesBatched is the shape the pump actually produces under load, where
// the drain hands over whatever the 256-deep log channel had buffered.

// logLine is a representative streamed line: long enough that measuring it is not free.
func logLine(i int) string {
	return "2026-07-29T10:00:00Z pod/api handled request " + strconv.Itoa(i)
}

func benchView() Model {
	m := New(styles.Default())
	m.SetSize(120, 40)
	m.Show()
	return m
}

func BenchmarkStreamLines(b *testing.B) {
	for _, n := range []int{1000, 4000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for range b.N {
				m := benchView()
				for i := range n {
					m.Append("", logLine(i))
				}
			}
		})
	}
}

// batchSize mirrors the kube layer's log channel buffer (kube.logChanBuffer): under load
// the drain finds roughly that many lines waiting and delivers them as one batch.
const batchSize = 256

func BenchmarkStreamLinesBatched(b *testing.B) {
	for _, n := range []int{1000, 4000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			for range b.N {
				m := benchView()
				batch := make([]Line, 0, batchSize)
				for i := range n {
					batch = append(batch, Line{Message: logLine(i)})
					if len(batch) == batchSize {
						m.AppendBatch(batch)
						batch = batch[:0]
					}
				}
				m.AppendBatch(batch)
			}
		})
	}
}
