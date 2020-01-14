package grip

import (
	"bytes"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type Transport interface {
	NewConnection(path string, id string) *Connection

	ForwardRequest(w http.ResponseWriter, r *http.Request)

	sendEvents(path string, connectionID string, e []Event) (http.Header, []Event, error)
}

type HTTPTransport struct {
	endpoint string
	signer   *Signer
	proxy    http.Handler
}

func NewHTTPTransport(endpoint string, signer *Signer) (*HTTPTransport, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	return &HTTPTransport{
		endpoint: endpoint,
		signer:   signer,
		proxy:    httputil.NewSingleHostReverseProxy(u),
	}, nil
}

func (t *HTTPTransport) NewConnection(path string, id string) *Connection {
	return &Connection{
		transport: t,
		path:      path,
		id:        id,
	}
}

func (t *HTTPTransport) ForwardRequest(w http.ResponseWriter, r *http.Request) {
	t.proxy.ServeHTTP(w, r)
}

func (t *HTTPTransport) sendEvents(path string, connectionID string, outgoingEvents []Event) (http.Header, []Event, error) {
	var buf bytes.Buffer
	for _, event := range outgoingEvents {
		if err := WriteEvent(&buf, event); err != nil {
			return nil, nil, err
		}
	}

	req, err := http.NewRequest(http.MethodPost, t.endpoint+path, &buf)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Add("Connection-Id", connectionID)
	req.Header.Add("Content-Type", "application/websocket-events")

	if t.signer != nil {
		sig, err := t.signer.Sign(time.Now().Add(time.Hour))
		if err != nil {
			return nil, nil, err
		}

		req.Header.Add("Grip-Sig", sig)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}

	newMetadata := map[string]string{}

	for k, vs := range res.Header {
		if len(k) <= 9 || !strings.HasPrefix(k, "Set-Meta-") {
			continue
		}

		for _, v := range vs {
			// last one takes precidence
			newMetadata[k[:9]] = v
		}
	}

	defer res.Body.Close()

	var incomingEvents []Event
	for it := NewEventIterator(res.Body); ; {
		event, err := it.Next()
		if err == Done {
			break
		} else if err != nil {
			return res.Header, nil, err
		}

		incomingEvents = append(incomingEvents, event)
	}

	return res.Header, incomingEvents, nil
}

type Connection struct {
	transport Transport
	path      string
	id        string
}

func (c *Connection) SendEvents(e ...Event) (http.Header, []Event, error) {
	return c.transport.sendEvents(c.path, c.id, e)
}
