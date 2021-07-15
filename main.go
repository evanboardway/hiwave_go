package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/evanboardway/hiwave_go/controllers"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

func handleRequests() {

	hiwaveRouter := mux.NewRouter().StrictSlash(true)
	hiwaveRouter.HandleFunc("/connect", controllers.InitPeer).Methods("POST")

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
