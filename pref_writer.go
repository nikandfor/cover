package main

import (
	"bytes"

	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/low"
)

type PrefWriter struct {
	b     []byte
	rem   []byte
	pref  [][]byte
	color [][]byte
	mode  int
	mid   bool
}

const (
	Reset = iota
	Gray
	Green
	Red
)

func (w *PrefWriter) Write(p []byte) (n int, err error) {
	if w.color != nil {
		w.b = append(w.b, w.color[w.mode]...)
	}

	if tlog.If("boundaries") {
		w.b = low.AppendPrintf(w.b, "|%d:%d|", len(p), w.mode)
	}

	for n < len(p) {
		if !w.mid && w.pref != nil {
			w.b = append(w.b, w.pref[w.mode]...)
			w.mid = true
		}

		i := bytes.IndexByte(p[n:], '\n')
		i++ // including

		if i == 0 {
			i = len(p)
		} else {
			i = n + i
			w.mid = false
		}

		w.b = append(w.b, p[n:i]...)

		n = i
	}

	return
}

func (w *PrefWriter) Close() error {
	if w.mode != Reset && w.color != nil {
		w.b = append(w.b, w.color[Reset]...)
	}

	w.mode = Reset

	return nil
}
