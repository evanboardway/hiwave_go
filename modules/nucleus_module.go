package modules

import (
	"log"

	"github.com/evanboardway/hiwave_go/types"
)

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
