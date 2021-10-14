package modules

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

type Client struct {
	// a pointer to the nucleus so that we can access its channels.
	Nucleus *Nucleus

	// The identifier that the client is stored as in the Nucleus.
	UUID uuid.UUID

	// websocket connection
	Socket *types.ThreadSafeWriter

	// a channel whose data is written to the websocket.
	WriteChan chan *types.WebsocketMessage

	// a track referencing audio packets being sent from the client.
	InboundAudio *webrtc.TrackLocalStaticRTP

	// a reference to the peers peer connection object.
	PeerConnection *webrtc.PeerConnection
}

func NewClient(safeConn *types.ThreadSafeWriter, nucleus *Nucleus) *Client {
	log.Printf("New client")
	return &Client{
		UUID:      uuid.New(),
		Socket:    safeConn,
		WriteChan: make(chan *types.WebsocketMessage, 10),
		Nucleus:   nucleus,
	}
}

func Reader(client *Client) {

	// If the loop ever breaks (can no longer read from the client socket)
	// we remove the client from the nucleus and close the socket.
	defer func() {
		// peer connection close
		client.Nucleus.Unsubscribe <- client
		client.Socket.Conn.Close()
	}()

	// Read message from the socket, determine where it should go.
	for {
		message := &types.WebsocketMessage{}

		// Lock the mutex, read from the socket, unlock the mutex.
		client.Socket.Mutex.RLock()
		_, raw, err := client.Socket.Conn.ReadMessage()
		client.Socket.Mutex.RUnlock()

		// Handle errors on message read, decode the raw messsage.
		if err != nil {
			log.Printf("Error reading")
			log.Printf("%+v\n", err)
			break
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Printf("%s", raw)
			log.Printf("Error unmarshaling")
			log.Printf("%+v\n", err)
		}
		log.Printf("New message from %s: %+v", client.UUID, message)

		switch message.Event {
		case "wrtc_connect":
			// init peer connection and send them an offer
			CreatePeerConnection(client)
		case "wrtc_answer":
			fmt.Printf("%+v\n", message.Data)
		}
	}
}

// read and write to socket asynchronously
func Writer(client *Client) {
	defer func() {
		client.Socket.Conn.Close()
	}()

	for {
		if len(client.WriteChan) > 0 {
			temp := <-client.WriteChan
			log.Printf("Writing to client, %+v", temp.Event)
			err := client.Socket.WriteJSON(temp)
			if err != nil {
				fmt.Printf("Write error %+v", err)
			}
		}
		// what happens if write fails?
	}
}

func CreatePeerConnection(client *Client) {
	// Configure ICE servers
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.stunprotocol.org"},
			},
		},
	}

	// Create new PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("%+v\n", err)
	}

	// Trickle ICE handler
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		candidateString, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			fmt.Printf("%+v\n", err)
			return
		}

		client.WriteChan <- &types.WebsocketMessage{
			Event: "wrtc_candidate",
			Data:  string(candidateString),
		}
	})

	// If the peer connection fails...
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			if closeErr := peerConnection.Close(); closeErr != nil {
				panic(closeErr)
			}
		}
	})

	// Create an offer with our current config
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}

	// Set the local description.
	if err = peerConnection.SetLocalDescription(offer); err != nil {
		fmt.Printf("%+v\n", err)
	}

	// Convert offer to json string
	offerString, err := json.Marshal(offer)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}

	// Write the offer to the client.
	client.Socket.WriteJSON(&types.WebsocketMessage{
		Event: "wrtc_offer",
		Data:  string(offerString),
	})

	client.PeerConnection = peerConnection

}
