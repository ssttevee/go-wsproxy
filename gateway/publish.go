package gateway

import (
	"encoding/json"
	"log"
	"net/http"
)

type websocketMessage struct {
	Content    *string `json:"content"`
	ContentBin []byte  `json:"content-bin"`
}

type controlItemFormats struct {
	WSMessage *websocketMessage `json:"ws-message"`

	// TODO: implement http-stream and http-response
}

type controlItem struct {
	Action  *string             `json:"action"`
	Channel string              `json:"channel"`
	ID      *string             `json:"id"`
	Formats *controlItemFormats `json:"formats"`
	Code    *uint16             `json:"code"`
}

type envelopedControlItems struct {
	Items []*controlItem `json:"items"`
}

func (g *Gateway) handlePublishRequest(w http.ResponseWriter, r *http.Request) {
	var data envelopedControlItems
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Println("failed to decode publish payload:", err)
		return
	}

}
