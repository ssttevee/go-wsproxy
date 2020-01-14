package gateway

import (
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/ssttevee/go-wsproxy/grip"
)

type Gateway struct {
	t grip.Transport

	mu          sync.RWMutex
	connections map[uuid.UUID]*Connection
	channels    map[string]map[*Connection]interface{}
}

func New(transport grip.Transport) *Gateway {
	return &Gateway{
		t: transport,
	}
}

func (g *Gateway) Forward(w http.ResponseWriter, r *http.Request) {
	g.t.ForwardRequest(w, r)
}

func (g *Gateway) Publish(channel string, mode string, content []byte) {
	ch, ok := g.channels[channel]
	if !ok {
		return
	}

	for c := range ch {
		go c.publishDataToClient(mode, content)
	}
}
