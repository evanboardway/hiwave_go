package modules

import (
	"github.com/evanboardway/hiwave_go/types"
	"github.com/gorilla/websocket"
)

type Client struct {
	// websocket connection
	Conn *websocket.Conn

	// a channel whose data is writeen to the websocket.
	WriteChan chan *types.WebsocketMessage

	// a channel that reads data from the socket
	ReadChan chan *types.WebsocketMessage
}

func NewClient(socket *websocket.Conn) *Client {
	return &Client{
		Conn:      socket,
		WriteChan: make(chan *types.WebsocketMessage, 1),
		ReadChan:  make(chan *types.WebsocketMessage, 2),
	}
}

// read and write to socket
