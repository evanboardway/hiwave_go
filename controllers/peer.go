package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

var (
	audioTrack *webrtc.TrackLocalStaticRTP
)

func InitPeer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Define configuration for peer objects. Defines ice server addresses.
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.stunprotocol.org"},
			},
		},
	}

	// Create new peer object with config
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}
	// defer func() {
	// 	if cErr := peerConnection.Close(); cErr != nil {
	// 		fmt.Printf("Cannot close peer connecton.\n %+v \n", cErr)
	// 	}
	// }()

	// create a local track
	audioTrack, newTrackErr := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "audio", "outbound")
	if newTrackErr != nil {
		panic(newTrackErr)
	}

	// add the treack to the connection
	rtpSender, err := peerConnection.AddTrack(audioTrack)
	if err != nil {
		panic(err)
	}

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
			fmt.Printf("%+v \n", rtcpBuf)
		}
	}()

	// Create a session description var to set as the body of the request
	var sessionDescription webrtc.SessionDescription

	fmt.Printf("Body: %+v\n", r)

	// Try to decode response body into the sessionDescription var.
	decodeErr := json.NewDecoder(r.Body).Decode(&sessionDescription)

	// Handle decoding error and respond to http client with a bad request.
	if decodeErr != nil {
		http.Error(w, decodeErr.Error(), http.StatusBadRequest)
		return
	}

	// set the remote description to the sessionDescription variable created earlier
	peerConnection.SetRemoteDescription(sessionDescription)

	// Handle the ontrack event for peer.
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {

		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for range ticker.C {
				errSend := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}})
				if errSend != nil {
					fmt.Println(errSend)
				}
			}
		}()

		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType(), track.Codec().MimeType)
		for {
			// Read RTP packets being sent to Pion
			rtp, _, readErr := track.ReadRTP()
			if readErr != nil {
				panic(readErr)
			}

			if writeErr := audioTrack.WriteRTP(rtp); writeErr != nil {
				panic(writeErr)
			}
		}

	})

	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

		if s == webrtc.PeerConnectionStateFailed {
			// Wait until PeerConnection has had no network activity for 30 seconds or another failure. It may be reconnected using an ICE Restart.
			// Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
			// Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
			fmt.Println("Peer Connection has gone to failed exiting")
			panic(s)
		}
	})

	// Create an answer with empty options
	answer, _ := peerConnection.CreateAnswer(nil)

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Set the local description to our response to the clients offer
	peerConnection.SetLocalDescription(answer)

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	js := make(map[string]string)
	js["type"] = "answer"
	js["sdp"] = answer.SDP

	response, marshErr := json.Marshal(js)

	if marshErr != nil {
		fmt.Printf(marshErr.Error())
	}

	fmt.Fprintf(w, "%s", response)
}
