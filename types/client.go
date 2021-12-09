package types

import (
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

type Client struct {
	// The identifier that the client is stored as in the Nucleus.
	UUID uuid.UUID

	// a pointer to the nucleus so that we can access its channels.
	Nucleus *Nucleus

	// Websocket connection
	Socket *ThreadSafeWriter

	// Client IP address used to ensure one connection per ip.
	IpAddr string

	// A channel whose data is written to the websocket.
	WriteChan chan *WebsocketMessage

	// A track referencing audio packets being sent from the client.
	InboundAudio chan []byte

	// A channel to stop routing audio to peers
	StopRoutingAudio chan bool

	// A channel to stop locate and connect function
	StopLAC chan bool

	// A chan to signal to the handleDisconnect when the locate and connect goroutine has been stopped
	StoppedLAC chan bool

	// A channel to signal when the client has been removed from the nucleus
	RemovedFromNucleus chan bool

	// A map of client uuid (key) to outbound audio tracks (value).
	RegisteredClients map[uuid.UUID]*AudioBundle

	// A reference to the peers peer connection object.
	PeerConnection *webrtc.PeerConnection

	// The clients current location
	CurrentLocation *LocationData

	// A mutex to lock a client so that only one resource can modify its peer connection at a time.
	PCMutex sync.RWMutex

	// A mutex to lock the registered clients list
	RCMutex sync.RWMutex
}

func NewClient(safeConn *ThreadSafeWriter, nucleus *Nucleus, remoteAddress string) *Client {
	log.Printf("New client")

	return &Client{
		UUID:               uuid.New(),
		Nucleus:            nucleus,
		Socket:             safeConn,
		IpAddr:             remoteAddress,
		WriteChan:          make(chan *WebsocketMessage),
		StopRoutingAudio:   make(chan bool),
		StopLAC:            make(chan bool),
		RemovedFromNucleus: make(chan bool),
		StoppedLAC:         make(chan bool),
		RegisteredClients:  make(map[uuid.UUID]*AudioBundle),
		InboundAudio:       make(chan []byte, 1500),
	}
}
