package grip

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
)

var Done = errors.New("done")

type EventIterator struct {
	r    io.Reader
	buf  bytes.Buffer
	tmp  []byte
	done bool
}

func NewEventIterator(r io.Reader) *EventIterator {
	return &EventIterator{
		r:   r,
		tmp: make([]byte, 1<<10),
	}
}

func (it *EventIterator) read() error {
	n, err := it.r.Read(it.tmp)
	if err != nil && (err != io.EOF || n == 0) {
		return err
	}

	_, _ = it.buf.Write(it.tmp[:n])

	return nil
}

func (it *EventIterator) Next() (Event, error) {
	if it.done && it.buf.Len() == 0 {
		return nil, Done
	}

	// read event header
	adv, line, err := bufio.ScanLines(it.buf.Bytes(), false)
	if err != nil {
		return nil, err
	}

	if adv == 0 {
		if err := it.read(); err == io.EOF {
			if it.buf.Len() > 0 {
				return nil, io.ErrUnexpectedEOF
			}

			it.done = true
			return nil, Done
		} else if err != nil {
			return nil, err
		}

		return it.Next()
	}

	// advance the buffer offset
	_ = it.buf.Next(adv)

	var contentLength int64
	if pos := bytes.IndexByte(line, ' '); pos != -1 {
		if contentLength, err = strconv.ParseInt(string(line[pos+1:]), 16, 64); err != nil {
			return nil, err
		}

		line = line[:pos]
	}

	for int(contentLength) > it.buf.Len() {
		if err := it.read(); err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		} else if err != nil {
			return nil, err
		}
	}

	var content []byte
	if contentLength > 0 {
		content = it.buf.Next(int(contentLength) + 2)[:int(contentLength)]
	}

	switch string(line) {
	case "OPEN", "PING", "PONG", "DISCONNECT":
		return EmptyEvent(line), nil

	case "TEXT", "BINARY":
		return DataEvent{
			t: string(line),
			p: content,
		}, nil

	case "CLOSE":
		if len(content) < 2 {
			return CloseEvent{}, nil
		}

		return CloseEvent{
			Code:   binary.BigEndian.Uint16(content),
			Reason: string(content[2:]),
		}, nil

	default:
		return nil, errors.New("unexpected event type: " + string(line))
	}
}
