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
	InboundAudio chan []byte

	// a reference to the peers peer connection object.
	PeerConnection *webrtc.PeerConnection
}

func NewClient(safeConn *types.ThreadSafeWriter, nucleus *Nucleus) *Client {
	log.Printf("New client")
	return &Client{
		UUID:         uuid.New(),
		Socket:       safeConn,
		WriteChan:    make(chan *types.WebsocketMessage),
		InboundAudio: make(chan []byte),
		Nucleus:      nucleus,
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
			log.Printf("Error unmarshaling: %+v", err)
		}
		log.Printf("New message from %s: %+v", client.UUID, message.Event)

		switch message.Event {
		case "wrtc_connect":
			// init peer connection and send them an offer
			createPeerConnection(client)
			break
		case "wrtc_offer":
			handleOffer(client, message)
			break

		case "wrtc_candidate":
			handleIceCandidate(client, message)
			break

		case "mute":
			outboundAudio, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "audio", "hiwave_go")
			if err != nil {
				panic(err)
			}

			if _, err := client.PeerConnection.AddTransceiverFromTrack(outboundAudio, webrtc.RTPTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendrecv}); err != nil {
				fmt.Println(err)
			}

			go func() {
				for {
					outboundAudio.Write(<-client.InboundAudio)
				}
			}()
			break

		case "voice":
			client.WriteChan <- &types.WebsocketMessage{
				Event: "test",
				Data:  "testing",
			}

		case "wrtc_renegotiation_needed":
			handleRenegotiation(client, message)
			break
		}
	}
}

// read and write to socket asynchronously
func Writer(client *Client) {
	defer func() {
		client.Socket.Conn.Close()
		client.Nucleus.Unsubscribe <- client
	}()

	for {
		temp := <-client.WriteChan
		log.Printf("Writing to client, %+v", temp.Event)
		err := client.Socket.WriteJSON(temp)
		if err != nil {
			log.Printf("Write error %+v", err)
		}
		// what happens if write fails?
	}
}

func createPeerConnection(client *Client) {
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

	peerConnection.OnNegotiationNeeded(func() {
		log.Println("PC EVENT: renegotiation needed")
		offer, err := peerConnection.CreateOffer(nil)
		if err != nil {
			log.Printf("Error renegotiating offer: %s", err)
		}

		if err = client.PeerConnection.SetLocalDescription(offer); err != nil {
			log.Printf("Error setting local description: %s", err)
		}

		off, err := json.Marshal(offer)
		if err != nil {
			log.Printf("Error marshaling renegotiation offer: %s", err)
		}

		client.WriteChan <- &types.WebsocketMessage{
			Event: "wrtc_renegotiation_needed",
			Data:  string(off),
		}
	})

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
		log.Printf("Connection State has changed to %s \n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			if closeErr := peerConnection.Close(); closeErr != nil {
				log.Println(closeErr)
			}
		}
	})

	peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Println("DATA CHAN")
	})

	peerConnection.OnTrack(func(tr *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		fmt.Println("ON TRACK")

		buff := make([]byte, 1500)
		for {
			i, _, err := tr.Read(buff)
			if err != nil {
				return
			}

			client.InboundAudio <- buff[:i]

			// if _, err = .Write(buff[:i]); err != nil {
			// 	return
			// }
		}
	})

	client.PeerConnection = peerConnection
}

func handleRenegotiation(client *Client, message *types.WebsocketMessage) {

	remoteOffer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &remoteOffer); err != nil {
		log.Print(err)
	}
	// fmt.Printf("HANDLE RENEG \n %+v \n", remoteOffer)

	if err := client.PeerConnection.SetRemoteDescription(remoteOffer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

}

func handleIceCandidate(client *Client, message *types.WebsocketMessage) {
	fmt.Printf("Candidate recvd: %+v", message)
	candidate := webrtc.ICECandidateInit{}
	if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	if err := client.PeerConnection.AddICECandidate(candidate); err != nil {
		fmt.Printf("%+v\n", err)
		return
	}
}

func handleOffer(client *Client, message *types.WebsocketMessage) {

	createPeerConnection(client)

	offer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &offer); err != nil {
		log.Print(err)
	}

	if err := client.PeerConnection.SetRemoteDescription(offer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

	answer, err := client.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Printf("Error creating answer: %s", err)
	}

	if err = client.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Printf("Error setting local description: %s", err)
	}

	ans, _ := json.Marshal(answer)

	client.WriteChan <- &types.WebsocketMessage{
		Event: "wrtc_answer",
		Data:  string(ans),
	}

}
