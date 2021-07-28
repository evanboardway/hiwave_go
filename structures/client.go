package structures

import (
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type Client struct {
	// Websocket connection to communicate
	conn *websocket.Conn

	// Peer connection object
	peerConnection *webrtc.PeerConnection
}

func NewClient(connection *websocket.Conn, peer *webrtc.PeerConnection) *Client {
	return &Client{
		conn:           connection,
		peerConnection: peer,
	}
}
