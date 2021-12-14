package main

import (
	"fmt"
	"log"
	"net/http"
	str "strings"
	"sync"

	"github.com/evanboardway/hiwave_go/modules"
	"github.com/evanboardway/hiwave_go/types"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	nucleus *types.Nucleus
)

func main() {
	// Clients will be registered in the nucleus. Information coming from the SFU will go through the nucleus.
	nucleus = types.CreateNucleus()

	go modules.Enable(nucleus)

	fmt.Println("Hiwave server started")

	// Connect to ws '/' for stats
	http.HandleFunc("/", statsHandler)

	http.HandleFunc("/websocket", websocketHandler)

	http.ListenAndServe(":5000", nil)
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("stats handler")
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Print("Error in upgrade:", err)
		return
	}

	go func() {
		for {
			select {
			case stat := <-nucleus.Stats:
				unsafeConn.WriteJSON(stat)
				break
			}
		}
	}()

}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Print("Error in upgrade:", err)
		return
	}

	remoteAddr := str.Split(r.RemoteAddr, ":")[0]

	nucleus.Mutex.RLock()
	for _, client := range nucleus.Clients {
		if client.IpAddr == remoteAddr {
			log.Printf("Prevented simultaneous connection from address %s with uuid %s\n %+v\n", remoteAddr, client.UUID, client)
			nucleus.Unsubscribe <- client
		}
	}
	nucleus.Mutex.RUnlock()

	// Wrap socket in a mutex that can lock the socket for write.
	safeConn := &types.ThreadSafeWriter{unsafeConn, sync.RWMutex{}}

	// Create a new client. Give it the socket and the nucleus's phone number
	newClient := types.NewClient(safeConn, nucleus, remoteAddr)

	// Start the write loop for the newly created client in a go routine.
	go modules.Writer(newClient)

	// Start the read loop for the newly created client in a go routine.
	go modules.Reader(newClient)

	// Tell the nucleus who the client is.
	nucleus.Subscribe <- newClient

}

// Client disconnects from audio:
// Unregister the client from everyone they're currrently registered to.
// Stop routing audio.
// Stop locate and connect.

// Client disconnects from server.
// Unsubscribe them from the nucleus (do this first to prevent other clients from registering to them)
// Unregister the client from everyone they're currently registered to (if pc exists)
// Stop routing audio.
// Stop locate and connect

// Client connects to server
// Subscribe them to nucleus

// Client connects to audio
// Start routing audio
// Start locate and connect
