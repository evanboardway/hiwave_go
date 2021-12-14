package types

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

	// A channel for pumping statistics through
	Stats chan string

	// A map of clients subscribed to the hub
	Clients map[uuid.UUID]*Client

	// Mutex to make sub and unsub chans one user only
	Mutex sync.RWMutex
}

// Create a nucleus and return a pointer to it.
func CreateNucleus() *Nucleus {
	log.Printf("New Nucleus")
	return &Nucleus{
		Subscribe:   make(chan *Client),
		Unsubscribe: make(chan *Client),
		Stats:       make(chan string, 1024),
		Clients:     make(map[uuid.UUID]*Client),
	}
}
