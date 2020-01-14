package gateway

import (
	"time"

	"github.com/gobwas/ws"
)

func (c *Connection) handleKeepAliveControlMessage(ctrl *controlMessage) {
	if ctrl.Timeout == nil || *ctrl.Timeout <= 0 {
		return
	}

	c.keepAliveMutex.Lock()
	defer c.keepAliveMutex.Unlock()

	c.keepAliveTimeout = time.Duration(*ctrl.Timeout) * time.Second

	if ctrl.MessageType == nil {
		ctrl.MessageType = new(string)
	}

	switch *ctrl.MessageType {
	case "binary":
		c.keepAliveMessageType = ws.OpBinary
		if len(ctrl.BinaryContent) == 0 {
			return
		}

		c.keepAliveContent = ctrl.BinaryContent
	case "ping":
		c.keepAliveMessageType = ws.OpPing
	case "pong":
		c.keepAliveMessageType = ws.OpPong
	case "":
		c.keepAliveMessageType = ws.OpText
		if ctrl.Content == nil || *ctrl.Content == "" {
			return
		}

		c.keepAliveContent = []byte(*ctrl.Content)
	default:
		return
	}

	if ctrl.Mode != nil && *ctrl.Mode == "interval" {
		c.keepAliveIntervalMode = true
	}

	c.keepAliveTimer = time.AfterFunc(c.keepAliveTimeout, c.sendKeepAlive)
}

func (c *Connection) sendKeepAlive() {
	c.keepAliveMutex.RLock()
	defer c.keepAliveMutex.RUnlock()

	c.keepAliveTimer.Stop()

	if c.keepAliveIntervalMode || c.isOutgoingMessageQueueEmpty() {
		c.keepAliveTimer.Reset(c.keepAliveTimeout)

		c.enqueueOutgoingMessage(c.keepAliveMessageType, c.keepAliveContent)
	}
}

func (c *Connection) maybeResetKeepAliveTimer() {
	c.keepAliveMutex.RLock()
	defer c.keepAliveMutex.RUnlock()

	c.keepAliveTimer.Stop()

	if c.keepAliveTimer != nil && c.keepAliveIntervalMode {
		c.keepAliveTimer.Reset(c.keepAliveTimeout)
	}
}
