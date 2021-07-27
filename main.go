package main

import (
	"fmt"
	"net/http"

	"github.com/evanboardway/hiwave_go/controllers"
)

func main() {
	fmt.Println("Hiwave server started")
	http.HandleFunc("/websocket", controllers.InitPeer)
	http.ListenAndServe(":5000", nil)
}
