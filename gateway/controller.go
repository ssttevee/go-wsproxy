package gateway

import (
	"bytes"
	"encoding/json"
)

var (
	defaultControlPrefix = []byte("c:")
	defaultMessagePrefix = []byte("m:")
)

type controlMessage struct {
	Type          string  `json:"type"`
	Channel       *string `json:"channel"`
	Content       *string `json:"content"`
	BinaryContent []byte  `json:"content-bin"`
	MessageType   *string `json:"message-type"`
	Timeout       *int    `json:"timeout"`
	Mode          *string `json:"mode"`
}

type controller struct {
	messagePrefix []byte
}

func (c *controller) isControl(content []byte) (*controlMessage, bool) {
	if !bytes.HasPrefix(content, defaultControlPrefix) {
		return nil, false
	}

	var ctrl controlMessage
	if err := json.Unmarshal(content[len(defaultControlPrefix):], &ctrl); err != nil {
		return nil, true
	}

	return &ctrl, true
}

func (c *controller) isMessage(content []byte) ([]byte, bool) {
	if len(c.messagePrefix) == 0 {
		return content, true
	}

	if !bytes.HasPrefix(content, c.messagePrefix) {
		return nil, false
	}

	return content[len(c.messagePrefix):], true
}

func (c *Connection) handleControlMessage(ctrl *controlMessage) {
	switch ctrl.Type {
	case "subscribe":
		if ctrl.Channel != nil && *ctrl.Channel != "" {
			c.subscribe(*ctrl.Channel)
		}
	case "unsubscribe":
		if ctrl.Channel != nil && *ctrl.Channel != "" {
			c.unsubscribe(*ctrl.Channel)
		}
	case "detach":
		c.detached.Store(true)
	case "keep-alive":
		c.handleKeepAliveControlMessage(ctrl)
	}
}
