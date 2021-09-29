package modules

// The nucleus is the place where each client is stored.
// The nucleus can write to each of the clients through their channels.

type Nucleus struct {
	// A channel to append clients to the nucleus
	Subscribe chan *Client

	// A channel to remove clients from the nucleus
	Unsubscribe chan *Client

	// A map of clients subscribed to the hub
	Clients map[*Client]struct{}
}

// Create a nucleus and return a pointer to it.
func CreateNucleus() *Nucleus {
	return &Nucleus{
		Subscribe:   make(chan *Client),
		Unsubscribe: make(chan *Client),
		Clients:     make(map[*Client]struct{}),
	}
}
