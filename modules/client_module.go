package modules

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

func Reader(client *types.Client) {
	fmt.Printf("%s reader started\n", client.UUID)

	// If the loop ever breaks (can no longer read from the client socket)
	// we remove the client from the nucleus and close the socket.
	defer func() {
		shutdownClient(client)
	}()

	// Read message from the socket, determine where it should go.
	for {
		message := &types.WebsocketMessage{}

		// Lock the mutex, read from the socket, unlock the mutex.
		client.Socket.Mutex.RLock()
		_, raw, err := client.Socket.Conn.ReadMessage()
		client.Socket.Mutex.RUnlock()

		// Handle errors on message read, decode the raw messsage.
		if err != nil {
			log.Printf("Error reading")
			log.Printf("%+v\n", err)
			break
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Printf("Error unmarshaling: %+v", err)
		}

		switch message.Event {
		case "wrtc_connect":
			createPeerConnection(client)
			break

		case "wrtc_offer":
			handleOffer(client, message)
			break

		case "wrtc_disconnect":
			handleDisconnect(client)
			break

		case "wrtc_answer":
			handleAnswer(client, message)
			break

		case "wrtc_candidate":
			handleIceCandidate(client, message)
			break

		case "voice":
			register(client, client)
			break

		case "mute":
			// unregister(client, client)
			shutdownClient(client)
			break

		case "wrtc_renegotiation_needed":
			handleRenegotiation(client, message)
			break

		case "update_location":
			updateClientLocation(client, message)
			break

		case "set_current_avatar":
			fmt.Printf("Avatar set %+v\n", message)
			client.Avatar = message.Data
			break
		}

	}
}

// Write to socket synchronously (unbuffered chan)
func Writer(client *types.Client) {
	fmt.Printf("%s writer started\n", client.UUID)

	defer func() {
		shutdownClient(client)
	}()

	for {
		data := <-client.WriteChan
		err := client.Socket.Conn.WriteJSON(data)
		if err != nil {
			// if writing to the socket fails, function returns and defer block is called
			log.Printf("Write error %+v", err)
			return
		}
	}
}

func register(client *types.Client, registree *types.Client) {

	// add track to client, add track to global list of senders.
	newTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "sfu_audio", client.UUID.String())
	if err != nil {
		log.Println(err)
	}

	transceiver, err := registree.PeerConnection.AddTransceiverFromTrack(newTrack, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly})
	if err != nil {
		log.Println(err)
	}

	audioBundle := &types.AudioBundle{
		Transceiver: transceiver,
		Track:       newTrack,
	}

	client.RCMutex.Lock()
	client.RegisteredClients[registree.UUID] = audioBundle
	client.RCMutex.Unlock()
	log.Printf("Registered client %s to client %s\n", registree.UUID, client.UUID)
}

func unregister(client *types.Client, unregistree *types.Client) {

	client.RCMutex.Lock()
	unregistreeBundle := client.RegisteredClients[unregistree.UUID]
	delete(client.RegisteredClients, unregistree.UUID)
	client.RCMutex.Unlock()

	log.Printf("Unregistree audio bundle: %+v cli: %s\n", unregistreeBundle, client.UUID)

	if err := unregistree.PeerConnection.RemoveTrack(unregistreeBundle.Transceiver.Sender()); err != nil {
		log.Printf("Error removing track on unregistree peer connection %s\n", err)
	}

	unregistreeBundle.Transceiver.Stop()

	log.Printf("Unregistered client %s from client %s\n", unregistree.UUID, client.UUID)

	client.WriteChan <- &types.WebsocketMessage{
		Event: "wrtc_remove_stream",
		Data:  unregistree.UUID.String(),
	}
}

// Register and unregister peers to this clients's audio streams based on relative location.
func locateAndConnect(client *types.Client) {
	defer func() {
		log.Printf("Client %s stopped LAC\n", client.UUID)
	}()
	for {
		select {
		case <-client.StopLAC:
			client.StoppedLAC <- true
			return
		default:
			// Compile a list of all the clients that are connected and not the same as the client param.
			client.Nucleus.Mutex.RLock()
			filtered_clients := make(map[uuid.UUID]*types.Client)
			for _, peer := range client.Nucleus.Clients {
				peer.PCMutex.RLock()
				if peer.PeerConnection != nil && peer.UUID != client.UUID {
					filtered_clients[peer.UUID] = peer
				}
				peer.PCMutex.RUnlock()
			}
			client.Nucleus.Mutex.RUnlock()

			// Add a check to make sure that the peer has a peer connection object
			for peer_uuid, peer := range filtered_clients {
				within_range := types.WithinRange(client.CurrentLocation, peer.CurrentLocation)

				client.RCMutex.RLock()
				registered := client.RegisteredClients[peer_uuid]
				client.RCMutex.RUnlock()

				if registered != nil {
					if within_range == false {
						client.WriteChan <- &types.WebsocketMessage{
							Event: "peer",
							Data:  "disconnected peer" + peer.UUID.String(),
						}
						unregister(client, peer)
					}
				} else if within_range {
					client.WriteChan <- &types.WebsocketMessage{
						Event: "peer",
						Data:  "connected peer" + peer.UUID.String(),
					}
					register(client, peer)
				}
			}
		}
	}
}

func RouteAudioToClients(client *types.Client) {
	for {
		select {
		case packet := <-client.InboundAudio:
			client.RCMutex.RLock()
			for _, registreeBundle := range client.RegisteredClients {
				registreeBundle.Track.Write(packet)
			}
			client.RCMutex.RUnlock()
			break
		case <-client.StopRoutingAudio:
			return
		}

	}
}

func updateClientLocation(client *types.Client, message *types.WebsocketMessage) {

	location := &types.LocationData{}

	if err := json.Unmarshal([]byte(message.Data), &location); err != nil {
		log.Print(err)
	}

	bundle := &types.LocationBundle{
		UUID:     client.UUID,
		Location: location,
		Avatar:   client.Avatar,
	}

	locationMarshaled, err := json.Marshal(bundle)
	if err != nil {
		log.Printf("Error marshaling client location: %s", err)
	}

	client.CurrentLocation = location

	client.Nucleus.Mutex.RLock()
	for uuid, peer := range client.Nucleus.Clients {
		if uuid != client.UUID {
			peer.WriteChan <- &types.WebsocketMessage{
				Event: "peer_location",
				Data:  string(locationMarshaled),
			}
		}
	}
	client.Nucleus.Mutex.RUnlock()
}

func handleDisconnect(client *types.Client) {
	// Signal locate and connect goroutine to shutdown
	client.StopLAC <- true

	// Wait until the clients locate and connect goroutine signals that it has stopped
	<-client.StoppedLAC

	client.PCMutex.Lock()

	client.StopRoutingAudio <- true

	// Make a copy of registered clients.
	RegisteredClientsCopy := make(map[uuid.UUID]*types.Client)

	client.RCMutex.RLock()
	for uuid := range client.RegisteredClients {
		RegisteredClientsCopy[uuid] = client.Nucleus.Clients[uuid]
	}
	client.RCMutex.RUnlock()

	// Unregister all currently registered clients from this one.
	for uuid := range RegisteredClientsCopy {
		unregister(client.Nucleus.Clients[uuid], client)
	}

	client.PeerConnection.Close()

	client.PeerConnection = nil

	client.PCMutex.Unlock()
}

func shutdownClient(client *types.Client) {
	log.Printf("Shutting down client %s\n", client.UUID)

	// Unsubscribe this client from the nucleus.
	client.Nucleus.Unsubscribe <- client

	// Wait until the nucleus signals that the client has been removed
	temp := <-client.RemovedFromNucleus

	log.Printf("%s Removed from nucleus, %+v", client.UUID, temp)

	client.Nucleus.Mutex.RLock()
	for uuid, peer := range client.Nucleus.Clients {
		if uuid != client.UUID {
			peer.WriteChan <- &types.WebsocketMessage{
				Event: "peer_disconnected",
				Data:  client.UUID.String(),
			}
		}
	}
	client.Nucleus.Mutex.RUnlock()

	client.Socket.Conn.Close()
}
