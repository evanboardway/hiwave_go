package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v2"
)

var peers []*webrtc.PeerConnection

func handleRequests() {

	hiwaveRouter := mux.NewRouter().StrictSlash(true)
	hiwaveRouter.HandleFunc("/connect", initPeer).Methods("POST")

	log.Fatal(
		http.ListenAndServe(":5000",
			handlers.CORS(
				handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization", "Access-Control-Allow-Origin"}),
				handlers.AllowedMethods([]string{"GET", "POST", "PUT", "HEAD", "OPTIONS"}),
				handlers.AllowedOrigins([]string{"*"}))(hiwaveRouter)))
}

func main() {
	fmt.Println("Hiwave server started")
	handleRequests()
}
