package types

import (
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
	Mutex sync.RWMutex
}
