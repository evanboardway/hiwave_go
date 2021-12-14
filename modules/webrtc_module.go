package modules

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/pion/webrtc/v3"
)

func createPeerConnection(client *types.Client) {
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
		// log.Printf("PC EVENT: renegotiation needed, %s \n", peerConnection.SignalingState())

		if peerConnection.SignalingState() != webrtc.SignalingStateStable {
			log.Println("Blocked renegotiation due to pending offer")
			return
		}

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

		if connectionState == webrtc.ICEConnectionStateFailed {
			client.WriteChan <- &types.WebsocketMessage{
				Event: "wrtc_failed",
			}
			if closeErr := peerConnection.Close(); closeErr != nil {
				log.Printf("Error closing the peer connection %s", closeErr)
			}

			log.Println("Peer connection state failed. Handling client disconnect")
			handleDisconnect(client)
		}
	})

	peerConnection.OnTrack(func(tr *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		buff := make([]byte, 1500)
		for {
			i, _, err := tr.Read(buff)
			if err != nil {
				return
			}
			client.InboundAudio <- buff[:i]
		}
	})

	// Send incoming audio packets to all clients registered to this client.
	go RouteAudioToClients(client)

	client.PCMutex.Lock()
	client.PeerConnection = peerConnection
	client.PCMutex.Unlock()

	// Start locating and connecting to other clients.
	go locateAndConnect(client)
}

func handleRenegotiation(client *types.Client, message *types.WebsocketMessage) {

	remoteOffer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &remoteOffer); err != nil {
		log.Print(err)
	}

	if client.PeerConnection.SignalingState() != webrtc.SignalingStateStable {
		client.PeerConnection.SetLocalDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeRollback,
		})
		client.PeerConnection.SetRemoteDescription(remoteOffer)
	}

	if err := client.PeerConnection.SetRemoteDescription(remoteOffer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

	answer, err := client.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Printf("Error creating renegotiation answer %s", err)
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

func handleIceCandidate(client *types.Client, message *types.WebsocketMessage) {
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

func handleOffer(client *types.Client, message *types.WebsocketMessage) {

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

func handleAnswer(client *types.Client, message *types.WebsocketMessage) {
	answer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
		log.Print(err)
	}

	if err := client.PeerConnection.SetRemoteDescription(answer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

}
