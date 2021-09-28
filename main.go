package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// addr = flag.String("addr", "localhost:8080", "http service address")
	test = false
)

func main() {
	fmt.Println("Hiwave server started")
	http.HandleFunc("/websocket", initPeer)
	http.ListenAndServe(":5000", nil)
}

func initPeer(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Print("Error in upgrade:", err)
		return
	}

	safeConn := &types.ThreadSafeWriter{unsafeConn, sync.Mutex{}}
	defer safeConn.Conn.Close()

	asd, err := json.Marshal("testing")

	// Example message construction
	message := types.WebsocketMessage{
		Event: "test",
		Data:  string(asd),
	}
	for {
		time.Sleep(2 * time.Second)
		safeConn.Conn.WriteJSON(message)
	}
}
