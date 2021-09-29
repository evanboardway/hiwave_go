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
	nucleus := modules.CreateNucleus()
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

	safeConn := &types.ThreadSafeWriter{unsafeConn, sync.Mutex{}}
	defer safeConn.Conn.Close()

}
