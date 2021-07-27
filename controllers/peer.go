package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

type threadSafeWriter struct {
	*websocket.Conn
	sync.Mutex
}

func (t *threadSafeWriter) WriteJSON(v interface{}) error {
	t.Lock()
	defer t.Unlock()

	return t.Conn.WriteJSON(v)
}

func InitPeer(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Print("upgrade:", err)
		return
	}

	c := &threadSafeWriter{unsafeConn, sync.Mutex{}}

	// When this frame returns close the Websocket
	defer c.Close() //nolint

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
		return
	}

	// When this frame returns close the PeerConnection
	defer peerConnection.Close() //nolint

	// Accept an incoming audio track
	if _, err := peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		fmt.Printf("%+v\n", err)
		return
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

		if writeErr := c.WriteJSON(&websocketMessage{
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
		// Create a new TrackLocal with the same codec as our incoming
		trackLocal, err := webrtc.NewTrackLocalStaticRTP(tr.Codec().RTPCodecCapability, tr.ID(), tr.StreamID())
		if err != nil {
			panic(err)
		}

		// Add the created track to the peerConnection for loopback
		peerConnection.AddTrack(trackLocal)

		buff := make([]byte, 1500)
		for {
			i, _, err := tr.Read(buff)
			if err != nil {
				return
			}

			if _, err = trackLocal.Write(buff[:i]); err != nil {
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
	if err = c.Conn.WriteJSON(&websocketMessage{
		Event: "offer",
		Data:  string(offerString),
	}); err != nil {
		fmt.Printf("%+v\n", err)
	}

	// Read messages coming from the socket on a loop and handle accordingly.
	message := &websocketMessage{}
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			fmt.Printf("%+v\n", err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			fmt.Printf("%+v\n", err)
			return
		}

		fmt.Println(message)

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

}

// func InitPeer(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Content-Type", "application/json")

// // Define configuration for peer objects. Defines ice server addresses.
// config := webrtc.Configuration{
// 	ICEServers: []webrtc.ICEServer{
// 		{
// 			URLs: []string{"stun:stun.stunprotocol.org"},
// 		},
// 	},
// }

// 	// Create new peer object with config
// 	peerConnection, err := webrtc.NewPeerConnection(config)
// 	if err != nil {
// 		panic(err)
// 	}
// 	// defer func() {
// 	// 	if cErr := peerConnection.Close(); cErr != nil {
// 	// 		fmt.Printf("Cannot close peer connecton.\n %+v \n", cErr)
// 	// 	}
// 	// }()

// 	// create a local audio track to receive
// 	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeAudio} {
// 		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
// 			Direction: webrtc.RTPTransceiverDirectionRecvonly,
// 		}); err != nil {
// 			fmt.Print(err)
// 			return
// 		}
// 	}

// 	trackLocal, _ := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "someid", "hiwave_go")
// 	peerConnection.AddTrack(trackLocal)

// 	// Create a session description var to set as the body of the request
// 	var sessionDescription webrtc.SessionDescription

// 	// Try to decode response body into the sessionDescription var.
// 	decodeErr := json.NewDecoder(r.Body).Decode(&sessionDescription)

// 	fmt.Printf("Body: %+v\n", sessionDescription)

// 	// Handle decoding error and respond to http client with a bad request.
// 	if decodeErr != nil {
// 		http.Error(w, decodeErr.Error(), http.StatusBadRequest)
// 		return
// 	}

// 	// set the remote description to the sessionDescription variable created earlier
// 	peerConnection.SetRemoteDescription(sessionDescription)

// 	// Handle the ontrack event for peer.
// 	peerConnection.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
// 		// Create a track to fan out our incoming video to all peers
// 		trackLocal, _ := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
// 		peerConnection.AddTrack(trackLocal)

// 		buf := make([]byte, 1500)
// 		for {
// 			i, _, err := t.Read(buf)
// 			if err != nil {
// 				return
// 			}

// 			if _, err = trackLocal.Write(buf[:i]); err != nil {
// 				return
// 			}
// 		}
// 	})

// 	// Set the handler for Peer connection state
// 	// This will notify you when the peer has connected/disconnected
// 	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
// 		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

// 	})

// 	// Create an answer with empty options
// 	answer, _ := peerConnection.CreateAnswer(nil)

// 	// Create channel that is blocked until ICE Gathering is complete
// 	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

// 	// Set the local description to our response to the clients offer
// 	peerConnection.SetLocalDescription(answer)

// 	// Block until ICE Gathering is complete, disabling trickle ICE
// 	// we do this because we only can exchange one signaling message
// 	// in a production application you should exchange ICE Candidates via OnICECandidate
// 	<-gatherComplete

// js := make(map[string]string)
// js["type"] = "answer"
// js["sdp"] = answer.SDP

// response, marshErr := json.Marshal(js)

// if marshErr != nil {
// 	fmt.Printf(marshErr.Error())
// }

// fmt.Fprintf(w, "%s", response)
// }
