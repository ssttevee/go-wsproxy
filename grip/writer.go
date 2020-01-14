package grip

import (
	"bytes"
	"io"
	"strconv"
)

var eol = []byte{13, 10}

func WriteEvent(w io.Writer, e Event) error {
	var buf bytes.Buffer
	buf.WriteString(e.Type())

	if content := e.Content(); len(content) > 0 {
		buf.WriteByte(' ')
		buf.WriteString(strconv.FormatInt(int64(len(content)), 16))
		buf.Write(eol)
		buf.Write(content)
	}

	buf.Write(eol)

	_, err := buf.WriteTo(w)
	return err
}
