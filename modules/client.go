package modules

import (
	"encoding/json"
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

	// a pointer to the nucleus so that we can access its channels.
	Nucleus *Nucleus
}

func NewClient(safeConn *types.ThreadSafeWriter, nucleus *Nucleus) *Client {
	log.Printf("New client")
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
	// we remove the client from the nucleus and close the socket.
	defer func() {
		log.Printf("Reader unsub")
		client.Nucleus.Unsubscribe <- client
		client.Socket.Conn.Close()
	}()

	// Read message from the socket, determine where it should go.
	for {
		log.Printf("Reading")
		message := &types.WebsocketMessage{}
		// Lock the mutex, read from the socket, unlock the mutex.
		client.Socket.Mutex.RLock()
		_, raw, err := client.Socket.Conn.ReadMessage()
		client.Socket.Mutex.RUnlock()
		// Handle errors on read message, else decode the raw messsage.
		if err != nil {
			log.Printf("Error reading")
			log.Printf("%+v\n", err)
			break
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Printf("Error unmarshaling")
			log.Printf("%+v\n", err)
		}
		log.Printf("%+v", message)
	}
}

// read and write to socket asynchronously
func Writer(client *Client) {
	log.Printf("Writer")
	defer func() {
		log.Printf("Writer close soc")
		client.Socket.Conn.Close()
	}()

	for {
		if len(client.WriteChan) > 0 {
			client.Socket.WriteJSON(<-client.WriteChan)
		}
		// what happens if write fails?
	}
}
