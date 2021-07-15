package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pion/webrtc/v2"
)

var connectedPeers []*Peer

type Peer struct {
	connection webrtc.PeerConnection
	track      webrtc.Track
}

func newPeer(connection *webrtc.PeerConnection) *Peer {
	newPeer := Peer{connection: *connection}
	return &newPeer
}

func InitPeer(w http.ResponseWriter, r *http.Request) {
	// Define configuration for peer objects. Defines ice server addresses.
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.stunprotocol.org"},
			},
		},
	}

	// Create new peer object with config
	peer, _ := webrtc.NewPeerConnection(config)

	// Create a session description var to set as the body of the request
	var sessionDescription webrtc.SessionDescription

	// Try to decode response body into the sessionDescription var.
	err := json.NewDecoder(r.Body).Decode(&sessionDescription)

	// Handle decoding error and respond to http client with a bad request.
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// set the remote description to the sessionDescription variable created earlier
	peer.SetRemoteDescription(sessionDescription)

	// Create an answer with empty options
	answer, _ := peer.CreateAnswer(&webrtc.AnswerOptions{})

	// Set the local description to our response to the clients offer
	peer.SetLocalDescription(answer)

	// create a new peer object with our webrtc.NewPeerConnection object
	newPeer := newPeer(peer)

	// We need to keep a reference to their track so we can append it to another peers track.
	newPeer.connection.OnTrack(func(t *webrtc.Track, r *webrtc.RTPReceiver) {
		newPeer.track = *t
	})

	// Add our newly created peer to the connectedPeers array
	connectedPeers = append(connectedPeers, newPeer)

	fmt.Printf("%+v\n", sessionDescription)

	fmt.Fprintf(w, "%v", newPeer.connection.LocalDescription())
}
