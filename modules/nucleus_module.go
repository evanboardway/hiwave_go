package modules

import (
	"log"

	"github.com/evanboardway/hiwave_go/types"

	"github.com/google/uuid"
)

// Create a nucleus and return a pointer to it.
func CreateNucleus() *types.Nucleus {
	log.Printf("New Nucleus")
	return &types.Nucleus{
		Subscribe:   make(chan *types.Client),
		Unsubscribe: make(chan *types.Client),
		Clients:     make(map[uuid.UUID]*types.Client),
	}
}

func Enable(nucleus *types.Nucleus) {
	log.Printf("Nucleus enable")
	for {
		select {
		case sub := <-nucleus.Subscribe:
			nucleus.Mutex.Lock()
			nucleus.Clients[sub.UUID] = sub
			log.Printf("Connected clients: %+v\n", nucleus.Clients)
			nucleus.Mutex.Unlock()
			log.Printf("Subscribed client")
		case unsub := <-nucleus.Unsubscribe:
			nucleus.Mutex.Lock()
			delete(nucleus.Clients, unsub.UUID)
			log.Printf("Connected clients: %+v\n", nucleus.Clients)
			nucleus.Mutex.Unlock()
			unsub.RemovedFromNucleus <- true
			log.Printf("Unsubed client %s", unsub.UUID)
		}
	}
}
