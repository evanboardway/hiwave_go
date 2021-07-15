package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v2"
)

func handleRequests() {
	myRouter := mux.NewRouter().StrictSlash(true)
	myRouter.HandleFunc("/connect", initPeer)

	log.Fatal(http.ListenAndServe(":10000", myRouter))
}

func initPeer(w http.ResponseWriter, r *http.Request) {
	offerPc, _ := webrtc.NewPeerConnection(webrtc.Configuration{})
	answerPc, _ := webrtc.NewPeerConnection(webrtc.Configuration{})

	offerPc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			answerPc.AddICECandidate(c.ToJSON())
		}
	})

	answerPc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			offerPc.AddICECandidate(c.ToJSON())
		}
	})

	offer, _ := offerPc.CreateOffer(nil)
	offerPc.SetLocalDescription(offer)
	answerPc.SetRemoteDescription(offer)

	answer, _ := offerPc.CreateOffer(nil)
	answerPc.SetRemoteDescription(answer)
	offerPc.SetLocalDescription(answer)

	fmt.Fprintf(w, "%v", answer)
	fmt.Println("Endpoint Hit: homePage")
}

func main() {
	fmt.Println("Rest API v2.0 - Mux Routers")
	handleRequests()
}
