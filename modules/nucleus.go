package modules

import (
	"log"
	"sync"

	"github.com/google/uuid"
)

// The nucleus is the place where each client is stored.
// The nucleus can write to each of the clients through their channels.

type Nucleus struct {
	// A channel to append clients to the nucleus
	Subscribe chan *Client

	// A channel to remove clients from the nucleus
	Unsubscribe chan *Client

	// A map of clients subscribed to the hub
	Clients map[uuid.UUID]*Client

	// Mutex to make sub and unsub chans one user only
	Mutex sync.Mutex
}

// Create a nucleus and return a pointer to it.
func CreateNucleus() *Nucleus {
	log.Printf("New Nucleus")
	return &Nucleus{
		Subscribe:   make(chan *Client),
		Unsubscribe: make(chan *Client),
		Clients:     make(map[uuid.UUID]*Client),
	}
}

func Enable(nucleus *Nucleus) {
	log.Printf("Nucleus enable")
	go LocateAndConnect(nucleus)
	for {
		select {
		case sub := <-nucleus.Subscribe:
			nucleus.Mutex.Lock()
			nucleus.Clients[sub.UUID] = sub
			nucleus.Mutex.Unlock()
			log.Printf("Subscribed client")
		case unsub := <-nucleus.Unsubscribe:
			nucleus.Mutex.Lock()
			delete(nucleus.Clients, unsub.UUID)
			nucleus.Mutex.Unlock()
			log.Printf("Unsubed client")
		}
	}
}

// Register and unregister clients to eachothers audio streams based on location data.
func LocateAndConnect(nucleus *Nucleus) {
	for {
		if len(nucleus.Clients) == 2 {
			for _, member := range nucleus.Clients {
				log.Printf("%+v", member)
			}
			return
		}
	}
}
