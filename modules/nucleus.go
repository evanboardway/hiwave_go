package modules

import (
	"log"
	"math"
	"sync"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/google/uuid"
)

var (
	// 1/3 mile in terms of geographical coordinates
	ONE_THIRD_MILE = 0.00483091787
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
		nucleus.Mutex.Lock()
		filtered_clients := make(map[uuid.UUID]*Client)
		for _, peer := range nucleus.Clients {
			peer.PCMutex.Lock()
			if peer.PeerConnection != nil {
				filtered_clients[peer.UUID] = peer
			}
			peer.PCMutex.Unlock()
		}
		nucleus.Mutex.Unlock()

		for member_uuid, member := range filtered_clients {
			for peer_uuid, peer := range filtered_clients {
				// Check that peer and member are different clients and
				// check that they arent already registered to eachother.
				if member_uuid != peer_uuid {
					calculated_distance := calculateDistanceBetweenPeers(member.CurrentLocation, peer.CurrentLocation)
					member.RCMutex.RLock()
					registered := member.RegisteredClients[peer_uuid]
					member.RCMutex.RUnlock()
					if registered != nil {
						if calculated_distance > ONE_THIRD_MILE {
							member.WriteChan <- &types.WebsocketMessage{
								Event: "peer",
								Data:  "disconnected peer" + peer.UUID.String(),
							}
							peer.WriteChan <- &types.WebsocketMessage{
								Event: "peer",
								Data:  "disconnected peer" + member.UUID.String(),
							}
							member.Unregister <- peer
							peer.Unregister <- member
							delete(filtered_clients, peer_uuid)

						}
					} else if calculated_distance <= ONE_THIRD_MILE {
						member.WriteChan <- &types.WebsocketMessage{
							Event: "peer",
							Data:  "connected peer" + peer.UUID.String(),
						}
						peer.WriteChan <- &types.WebsocketMessage{
							Event: "peer",
							Data:  "connected peer" + member.UUID.String(),
						}
						member.Register <- peer
						peer.Register <- member
						delete(filtered_clients, peer_uuid)
					}
				}
			}
		}
	}
}

func calculateDistanceBetweenPeers(from *types.LocationData, to *types.LocationData) float64 {
	return math.Sqrt(math.Pow((to.Latitude-from.Latitude), 2) + math.Pow((to.Longitude-from.Longitude), 2))
}
