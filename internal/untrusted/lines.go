package untrusted

import "bytes"

// LineWriter hands each complete line written to it to Line, without its newline: output read as
// it comes (a stream of events or statuses). A line longer than Max is dropped whole; nothing
// sends one on purpose. The slice given to Line is valid only during the call.
type LineWriter struct {
	Max  int
	Line func([]byte)
	part []byte
	skip bool // the current line went over Max: drop it to its end
}

func (w *LineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if !w.skip {
				w.part = append(w.part, p...)
				if len(w.part) > w.Max {
					w.part, w.skip = nil, true
				}
			}
			break
		}
		if !w.skip && len(w.part)+i <= w.Max {
			w.Line(append(w.part, p[:i]...))
		}
		w.part, w.skip = w.part[:0], false
		p = p[i+1:]
	}
	return n, nil
}
