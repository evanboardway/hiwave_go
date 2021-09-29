package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/evanboardway/hiwave_go/modules"
	"github.com/evanboardway/hiwave_go/types"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	nucleus *modules.Nucleus
)

func main() {
	nucleus = modules.CreateNucleus()
	fmt.Println("Hiwave server started")
	http.HandleFunc("/websocket", websocketHandler)
	http.ListenAndServe(":5000", nil)
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Print("Error in upgrade:", err)
		return
	}

	// Wrap socket in a mutex that can lock the socket for write.
	safeConn := &types.ThreadSafeWriter{unsafeConn, sync.RWMutex{}}
	defer safeConn.Conn.Close()

	// Create a new client. Give it the socket and the nucleus's phone number
	newClient := modules.NewClient(safeConn, nucleus)

	// Start the write loop for the newly created client in a go routine.
	go modules.Writer(newClient)

	// Tell the nucleus who the client is.
	// nucleus.Subscribe <- newClient

	// Start the read loop for the newly created client in a go routine.
	go modules.Reader(newClient)

}
