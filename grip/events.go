package grip

import (
	"encoding/binary"
)

const (
	OpenEvent       EmptyEvent = "OPEN"
	PingEvent       EmptyEvent = "PING"
	PongEvent       EmptyEvent = "PONG"
	DisconnectEvent EmptyEvent = "DISCONNECT"
)

type Event interface {
	Type() string
	Content() []byte
}

type EmptyEvent string

func (e EmptyEvent) Type() string {
	return string(e)
}

func (e EmptyEvent) Content() []byte {
	return nil
}

type DataEvent struct {
	t string
	p []byte
}

func NewBinaryEvent(p []byte) Event {
	return DataEvent{
		t: "BINARY",
		p: p,
	}
}

func NewTextEvent(s string) Event {
	return DataEvent{
		t: "TEXT",
		p: []byte(s),
	}
}

func (e DataEvent) Type() string {
	return e.t
}

func (e DataEvent) Content() []byte {
	return e.p
}

func (e DataEvent) Text() string {
	return string(e.p)
}

func (e DataEvent) Bytes() []byte {
	return e.p
}

type CloseEvent struct {
	Code   uint16
	Reason string
}

func NewCloseEvent(code uint16, reason string) Event {
	return CloseEvent{
		Code:   code,
		Reason: reason,
	}
}

func (CloseEvent) Type() string {
	return "CLOSE"
}

func (e CloseEvent) Content() []byte {
	p := make([]byte, len(e.Reason)+2)
	binary.BigEndian.PutUint16(p, e.Code)
	copy(p[2:], e.Reason)
	return p
}
