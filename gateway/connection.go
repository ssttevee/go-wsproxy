package gateway

import (
	"encoding/binary"
	"io"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
	"github.com/ssttevee/go-wsproxy/grip"
	"go.uber.org/atomic"
)

type Connection struct {
	id uuid.UUID

	gw   *Gateway
	ctlr controller

	mu sync.Mutex
	rw io.ReadWriteCloser // connection
	fr *wsutil.Reader     // frame reader
	gc *grip.Connection

	// outgoing messages
	messages      []*wsutil.Message
	messagesMutex sync.RWMutex
	transmitable  atomic.Bool

	// outgoing events
	events        []grip.Event
	eventsMutex   sync.Mutex
	sendingEvents atomic.Bool

	opened   atomic.Bool
	closed   atomic.Bool
	detached atomic.Bool

	backendClosed atomic.Bool
	clientClosed  atomic.Bool

	keepAliveMutex        sync.RWMutex
	keepAliveTimer        *time.Timer
	keepAliveTimeout      time.Duration
	keepAliveIntervalMode bool
	keepAliveMessageType  ws.OpCode
	keepAliveContent      []byte

	close func()
}

func (g *Gateway) NewConnection(path string, conn io.ReadWriteCloser, onclose func()) *Connection {
	id := uuid.New()
	c := &Connection{
		id: id,
		ctlr: controller{
			messagePrefix: defaultMessagePrefix,
		},
		rw:    conn,
		fr:    wsutil.NewReader(conn, ws.StateServerSide),
		gc:    g.t.NewConnection(path, id.String()),
		close: onclose,
	}

	g.connections[c.id] = c

	c.enqueueOutgoingEvents(grip.OpenEvent)

	return c
}

func (c *Connection) Transmit() error {
	defer c.maybeResetKeepAliveTimer()

	opc, payload, ok := c.nextOutgoingMessage()
	if !ok {
		c.transmitable.Store(true)
		return nil
	}

	return wsutil.WriteServerMessage(c.rw, opc, payload)
}

func (c *Connection) Receive() error {
	h, err := c.fr.NextFrame()
	if err != nil {
		return err
	}

	payload, err := ioutil.ReadAll(c.fr)
	if err != nil {
		return err
	}

	switch h.OpCode {
	case ws.OpText:
		c.enqueueOutgoingEvents(grip.NewTextEvent(string(payload)))

	case ws.OpBinary:
		c.enqueueOutgoingEvents(grip.NewBinaryEvent(payload))

	case ws.OpClose:
		var code uint16
		if len(payload) > 1 {
			code = binary.BigEndian.Uint16(payload)
			payload = payload[2:]
		}

		c.enqueueOutgoingEvents(grip.CloseEvent{
			Code:   code,
			Reason: string(payload),
		})

		c.clientClosed.Store(true)

		if c.backendClosed.Load() {
			c.closed.Store(true)
			return c.Drop()
		}

	case ws.OpPing:
		c.enqueueOutgoingEvents(grip.PingEvent)

	case ws.OpPong:
		c.enqueueOutgoingEvents(grip.PongEvent)
	}

	return nil
}

func (c *Connection) Drop() error {
	if !c.closed.Load() {
		c.closed.Store(true)
		c.enqueueOutgoingEvents(grip.DisconnectEvent)
	}

	if c.close != nil {
		c.close()
	}

	return c.rw.Close()
}

func (c *Connection) handleIncomingEvents(events []grip.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, event := range events {
		if err := c.handleIncomingEventUnsafe(event); err != nil {
			return err
		}
	}

	return nil
}

func (c *Connection) isOutgoingMessageQueueEmpty() bool {
	c.messagesMutex.RLock()
	defer c.messagesMutex.RUnlock()

	return len(c.messages) == 0
}

func (c *Connection) nextOutgoingMessage() (_ ws.OpCode, _ []byte, ok bool) {
	c.messagesMutex.Lock()
	defer c.messagesMutex.Unlock()

	for len(c.messages) == 0 {
		return 0, nil, false
	}

	message := c.messages[0]
	c.messages[0] = nil
	c.messages = c.messages[1:]

	return message.OpCode, message.Payload, true
}

func (c *Connection) enqueueOutgoingMessage(opc ws.OpCode, payload []byte) {
	c.messagesMutex.Lock()
	defer c.messagesMutex.Unlock()

	c.messages = append(c.messages, &wsutil.Message{
		OpCode:  opc,
		Payload: payload,
	})

	if c.transmitable.CAS(true, false) {
		go c.Transmit()
	}
}

func (c *Connection) handleIncomingEvent(event grip.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.handleIncomingEventUnsafe(event)
}

func (c *Connection) handleIncomingEventUnsafe(event grip.Event) error {
	if c.closed.Load() {
		return nil
	}

	log.Println("RECV:", event.Type())

	if !c.opened.Load() {
		if event != grip.OpenEvent {
			log.Println("# first event must be OPEN but got", event.Type())
		}

		c.opened.Store(true)
		return nil
	}

	switch event {
	case grip.OpenEvent:
		// TODO consider complaining about multiple/superfluous open events
		return nil

	case grip.PingEvent:
		c.enqueueOutgoingMessage(ws.OpPing, nil)
		return nil

	case grip.PongEvent:
		c.enqueueOutgoingMessage(ws.OpPong, nil)
		return nil

	case grip.DisconnectEvent:
		// set closed so a disconnect event isn't sent back to the backend
		c.closed.Store(true)
		return c.Drop()
	}

	switch event := event.(type) {
	case grip.CloseEvent:
		c.enqueueOutgoingMessage(ws.OpClose, event.Content())

		c.backendClosed.Store(true)

		if c.clientClosed.Load() {
			c.closed.Store(true)
			return c.Drop()
		}

		return nil

	case grip.DataEvent:
		content := event.Content()
		if ctrl, ok := c.ctlr.isControl(content); ok {
			if ctrl != nil {
				c.handleControlMessage(ctrl)
			}
		} else if payload, ok := c.ctlr.isMessage(content); ok {
			c.publishDataToClient(event.Type(), payload)
		}

		return nil
	}

	panic("unreachable")
}

func (c *Connection) publishDataToClient(mode string, payload []byte) {
	switch mode {
	case "BINARY":
		c.enqueueOutgoingMessage(ws.OpBinary, payload)

	case "TEXT":
		c.enqueueOutgoingMessage(ws.OpText, payload)
	}
}

func (c *Connection) enqueueOutgoingEvents(events ...grip.Event) {
	if c.detached.Load() {
		return
	}

	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()

	for _, event := range events {
		log.Println("SEND:", event.Type())
	}

	c.events = append(c.events, events...)

	if c.sendingEvents.CAS(false, true) {
		c.sendingEvents.Store(true)

		go c.sendEventsToBackendLoop()
	}
}

func (c *Connection) nextEventBatch() []grip.Event {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()

	events := c.events
	c.events = nil
	return events
}

func (c *Connection) sendEventsToBackendLoop() {
	defer c.sendingEvents.Store(false)

	for {
		events := c.nextEventBatch()
		if len(events) == 0 {
			return
		}

		if err := c.sendEventsToBackend(events); err != nil {
			log.Println("# failed to send events to backend:", err)
			return
		}
	}
}

func (c *Connection) sendEventsToBackend(events []grip.Event) error {
	_, events, err := c.gc.SendEvents(events...)
	if err != nil {
		return err
	}

	return c.handleIncomingEvents(events)
}

func (c *Connection) subscribe(channel string) {
	ch, ok := c.gw.channels[channel]
	if !ok {
		ch = make(map[*Connection]interface{})
		c.gw.channels[channel] = ch
	}

	ch[c] = nil
}

func (c *Connection) unsubscribe(channel string) {
	ch, ok := c.gw.channels[channel]
	if !ok {
		return
	}

	delete(ch, c)

	if len(ch) == 0 {
		delete(c.gw.channels, channel)
	}
}
