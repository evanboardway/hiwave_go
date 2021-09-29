package modules

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/google/uuid"
)

type Client struct {
	// The identifier that the client is stored as in the Nucleus.
	UUID uuid.UUID

	// websocket connection
	Socket *types.ThreadSafeWriter

	// a channel whose data is writeen to the websocket.
	WriteChan chan *types.WebsocketMessage

	// a channel that reads data from the socket
	ReadChan chan *types.WebsocketMessage

	Nucleus *Nucleus
}

func NewClient(safeConn *types.ThreadSafeWriter, nucleus *Nucleus) *Client {
	return &Client{
		UUID:      uuid.New(),
		Socket:    safeConn,
		WriteChan: make(chan *types.WebsocketMessage, 1),
		ReadChan:  make(chan *types.WebsocketMessage, 1),
		Nucleus:   nucleus,
	}
}

func Reader(client *Client) {

	// If the loop ever breaks (can no longer read from the client socket)
	// we remove the client from the nucleus.
	defer func() {
		client.Nucleus.Unsubscribe <- client
	}()

	// Read message from the socket, determine where it should go.
	for {
		message := &types.WebsocketMessage{}
		// Lock the mutex, read from the socket, unlock the mutex.
		client.Socket.Mutex.RLock()
		_, raw, err := client.Socket.Conn.ReadMessage()
		client.Socket.Mutex.RUnlock()
		// Handle errors on read message, else decode the raw messsage.
		if err != nil {
			log.Printf("%+v\n", err)
			break
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Printf("%+v\n", err)
		}
		fmt.Printf("%+v", message)
	}
}

// read and write to socket asynchronously
func Writer(client *Client) {
	defer func() {
		client.Socket.Conn.Close()
	}()

	for {
		if len(client.WriteChan) > 0 {
			client.Socket.WriteJSON(<-client.WriteChan)
		}
		// what happens if write fails?
	}
}
