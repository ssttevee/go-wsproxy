package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/mailru/easygo/netpoll"
	"github.com/ssttevee/go-wsproxy/gateway"
	"github.com/ssttevee/go-wsproxy/grip"
)

var (
	addr      = flag.String("listen", ":8080", "address to bind to")
	ioTimeout = flag.Duration("io_timeout", time.Millisecond*100, "i/o operations timeout")
)

func main() {
	flag.Parse()

	// Initialize netpoll instance. We will use it to be noticed about incoming
	// events from listener of user connections.
	poller, err := netpoll.New(nil)
	if err != nil {
		log.Fatal(err)
	}

	transport, err := grip.NewHTTPTransport("http://localhost:12345", nil)
	if err != nil {
		log.Fatal(err)
	}

	chat := gateway.New(transport)

	// Create incoming connections listener.
	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("listening on %s", lis.Addr().String())

	http.Serve(lis, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Connection") == "Upgrade" && r.Header.Get("Upgrade") == "websocket" {
			conn, _, _, err := ws.UpgradeHTTP(r, w)
			if err != nil {
				log.Printf("# %s: upgrade error: %v", nameConn(conn), err)
				return
			}

			// Create netpoll event descriptor for conn.
			// We want to handle only read events of it.
			desc := netpoll.Must(netpoll.HandleReadWrite(conn))

			// Register incoming user in chat.
			user := chat.NewConnection(r.URL.Path, conn, func() {
				log.Printf("# %s: dropped", nameConn(conn))
				poller.Stop(desc)
			})

			// Subscribe to events about conn.
			poller.Start(desc, func(ev netpoll.Event) {
				log.Println(ev)

				if ev&(netpoll.EventReadHup|netpoll.EventHup) != 0 {
					// When ReadHup or Hup received, this mean that client has
					// closed at least write end of the connection or connections
					// itself. So we want to stop receive events about such conn
					// and remove it from the chat registry.
					user.Drop()
					return
				}

				if ev&netpoll.EventRead != 0 {
					go func() {
						if err := user.Receive(); err != nil {
							log.Println("# receive error:", err)
							// When receive failed, we can only disconnect broken
							// connection and stop to receive events about it.
							user.Drop()
						}
					}()
				}

				if ev&netpoll.EventWrite != 0 {
					go func() {
						if err := user.Transmit(); err != nil {
							log.Println("# transmit error:", err)
							// When receive failed, we can only disconnect broken
							// connection and stop to receive events about it.
							user.Drop()
						}
					}()
				}
			})
		} else {
			chat.Forward(w, r)
		}
	}))
}

func nameConn(conn net.Conn) string {
	return conn.LocalAddr().String() + " > " + conn.RemoteAddr().String()
}
