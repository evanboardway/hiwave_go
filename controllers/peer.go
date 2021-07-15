package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pion/webrtc/v2"
)

func initPeer(w http.ResponseWriter, r *http.Request) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.stunprotocol.org"},
			},
		},
	}

	peer, _ := webrtc.NewPeerConnection(config)

	var temp webrtc.SessionDescription

	err := json.NewDecoder(r.Body).Decode(&temp)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Printf("%+v\n", temp)

	fmt.Fprintf(w, "%v", "yeet")
}
