package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const (
	rtcpPLIInterval = time.Second * 3
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
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("Cannot close peer connecton.\n %+v \n", cErr)
		}
	}()

	// Allow us to receive 1 ausio track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		panic(err)
	}

	localTrackChan := make(chan *webrtc.TrackLocalStaticRTP)

	// Handle the ontrack event for peer.
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		go func() {
			ticker := time.NewTicker(rtcpPLIInterval)
			for range ticker.C {
				if rtcpSendErr := peerConnection.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())}}); rtcpSendErr != nil {
					fmt.Println(rtcpSendErr)
				}
			}
		}()

		// create a local track
		localTrack, newTrackErr := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, "audio", "outbound")
		if newTrackErr != nil {
			panic(newTrackErr)
		}
		localTrackChan <- localTrack

		rtpBuf := make([]byte, 1400)
		for {
			i, _, readErr := remoteTrack.Read(rtpBuf)
			if readErr != nil {
				panic(readErr)
			}

			// ErrClosedPipe means we don't have any subscribers, this is ok if no peers have connected yet
			if _, err = localTrack.Write(rtpBuf[:i]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				panic(err)
			}
		}
	})

	// Create a session description var to set as the body of the request
	var sessionDescription webrtc.SessionDescription

	// Try to decode response body into the sessionDescription var.
	decodeErr := json.NewDecoder(r.Body).Decode(&sessionDescription)

	// Handle decoding error and respond to http client with a bad request.
	if decodeErr != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// set the remote description to the sessionDescription variable created earlier
	peerConnection.SetRemoteDescription(sessionDescription)

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
