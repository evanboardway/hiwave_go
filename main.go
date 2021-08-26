package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	connectedClients []*Client
)

// This is our entry point to the application. Websocket listens and is handled by the controllers.
func main() {
	fmt.Println("Hiwave server started")
	http.HandleFunc("/websocket", init_peer)
	http.ListenAndServe(":5000", nil)
}

func init_peer(w http.ResponseWriter, r *http.Request) {

	// go func() {

	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Print("upgrade:", err)
		return
	}

	c := &types.ThreadSafeWriter{unsafeConn, sync.Mutex{}}

	// When this frame returns close the Websocket
	// defer c.Close() //nolint

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
		fmt.Printf("%+v\n", err)
	}

	// When this frame returns close the PeerConnection
	defer peerConnection.Close() //nolint

	// Append this client to our list of clients
	// connectedClients = append(connectedClients, types.NewClient(c.Conn, peerConnection))

	// Create a new TrackLocal
	outboundAudio, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "audio", "hiwave_go")
	if err != nil {
		panic(err)
	}

	// Accept an incoming audio track
	if _, err := peerConnection.AddTransceiverFromTrack(outboundAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendrecv}); err != nil {
		fmt.Println(err)
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

		if writeErr := c.WriteJSON(&types.WebsocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			fmt.Printf("%+v\n", writeErr)
		}
	})

	// Handle closing of peer conn
	peerConnection.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		fmt.Printf("State Change: %+v\n", pcs)
	})

	peerConnection.OnTrack(func(tr *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		fmt.Println("ONTRACK")

		localTracc, _ := webrtc.NewTrackLocalStaticRTP(tr.Codec().RTPCodecCapability, tr.ID(), tr.StreamID())

		buff := make([]byte, 1500)
		for {
			i, _, err := tr.Read(buff)
			if err != nil {
				return
			}

			if _, err = localTracc.Write(buff[:i]); err != nil {
				return
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

	// Write the offer to the socket
	if err = c.Conn.WriteJSON(&types.WebsocketMessage{
		Event: "offer",
		Data:  string(offerString),
	}); err != nil {
		fmt.Printf("%+v\n", err)
	}

	// Read messages coming from the socket on a loop and handle accordingly.
	message := &types.WebsocketMessage{}
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			fmt.Printf("%+v\n", err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			fmt.Printf("%+v\n", err)
			return
		}

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				fmt.Printf("%+v\n", err)
				return
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				fmt.Printf("%+v\n", err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				fmt.Printf("%+v\n", err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				fmt.Printf("%+v\n", err)
				return
			}
		}
	}

	// }()
}
