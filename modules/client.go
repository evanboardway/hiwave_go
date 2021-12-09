package modules

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/evanboardway/hiwave_go/types"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

type Client struct {
	// The identifier that the client is stored as in the Nucleus.
	UUID uuid.UUID

	// a pointer to the nucleus so that we can access its channels.
	Nucleus *Nucleus

	// Websocket connection
	Socket *types.ThreadSafeWriter

	// Client IP address used to ensure one connection per ip.
	IpAddr string

	// A channel whose data is written to the websocket.
	WriteChan chan *types.WebsocketMessage

	// A track referencing audio packets being sent from the client.
	InboundAudio chan []byte

	// A channel to stop routing audio to peers
	StopRoutingAudio chan bool

	// A channel to stop locate and connect function
	StopLAC chan bool

	// A chan to signal to the handleDisconnect when the locate and connect goroutine has been stopped
	StoppedLAC chan bool

	// A channel to signal when the client has been removed from the nucleus
	RemovedFromNucleus chan bool

	// A map of client uuid (key) to outbound audio tracks (value).
	RegisteredClients map[uuid.UUID]*types.AudioBundle

	// A reference to the peers peer connection object.
	PeerConnection *webrtc.PeerConnection

	// The clients current location
	CurrentLocation *types.LocationData

	// A mutex to lock a client so that only one resource can modify its peer connection at a time.
	PCMutex sync.RWMutex

	// A mutex to lock the registered clients list
	RCMutex sync.RWMutex
}

func NewClient(safeConn *types.ThreadSafeWriter, nucleus *Nucleus, remoteAddress string) *Client {
	log.Printf("New client")

	return &Client{
		UUID:               uuid.New(),
		Nucleus:            nucleus,
		Socket:             safeConn,
		IpAddr:             remoteAddress,
		WriteChan:          make(chan *types.WebsocketMessage),
		StopRoutingAudio:   make(chan bool),
		StopLAC:            make(chan bool),
		RemovedFromNucleus: make(chan bool),
		StoppedLAC:         make(chan bool),
		RegisteredClients:  make(map[uuid.UUID]*types.AudioBundle),
		InboundAudio:       make(chan []byte, 1500),
	}
}

func Reader(client *Client) {

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
			unregister(client, client)
			break

		case "wrtc_renegotiation_needed":
			handleRenegotiation(client, message)
			break

		case "update_location":
			updateClientLocation(client, message)
			break
		}

	}
}

// Write to socket synchronously (unbuffered chan)
func Writer(client *Client) {
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

func register(client *Client, registree *Client) {

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

	locationBundle := &types.LocationBundle{
		UUID:     client.UUID,
		Location: client.CurrentLocation,
	}

	loc, err := json.Marshal(locationBundle)
	if err != nil {
		log.Printf("Error marshaling client location %s\n", err)
	}

	client.WriteChan <- &types.WebsocketMessage{
		Event: "peer_location",
		Data:  string(loc),
	}

	client.RCMutex.Lock()
	client.RegisteredClients[registree.UUID] = audioBundle
	client.RCMutex.Unlock()
	log.Printf("Registered client %s to client %s\n", registree.UUID, client.UUID)
}

func unregister(client *Client, unregistree *Client) {

	// Peer connection needs to be in a stable state, lock the client mutex.
	client.RCMutex.RLock()
	unregistreeBundle := client.RegisteredClients[unregistree.UUID]
	client.RCMutex.RUnlock()

	log.Printf("Unregistree audio bundle: %+v cli: %s\n", unregistreeBundle, client.UUID)

	if err := unregistree.PeerConnection.RemoveTrack(unregistreeBundle.Transceiver.Sender()); err != nil {
		log.Printf("Error removing track on unregistree peer connection %s\n", err)
	}

	unregistreeBundle.Transceiver.Stop()

	client.RCMutex.Lock()
	delete(client.RegisteredClients, unregistree.UUID)
	client.RCMutex.Unlock()

	log.Printf("Unregistered client %s from client %s\n", unregistree.UUID, client.UUID)

	client.WriteChan <- &types.WebsocketMessage{
		Event: "wrtc_remove_stream",
		Data:  unregistree.UUID.String(),
	}
}

// Register and unregister peers to this clients's audio streams based on relative location.
func locateAndConnect(client *Client) {
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
			filtered_clients := make(map[uuid.UUID]*Client)
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

func RouteAudioToClients(client *Client) {
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

func updateClientLocation(client *Client, message *types.WebsocketMessage) {

	location := &types.LocationData{}

	if err := json.Unmarshal([]byte(message.Data), &location); err != nil {
		log.Print(err)
	}

	bundle := &types.LocationBundle{
		UUID:     client.UUID,
		Location: location,
	}

	client.CurrentLocation = location

	client.RCMutex.RLock()
	for uuid := range client.RegisteredClients {
		loc, err := json.Marshal(bundle)
		if err != nil {
			log.Printf("Error marshaling client location: %s", err)
		}
		client.Nucleus.Clients[uuid].WriteChan <- &types.WebsocketMessage{
			Event: "peer_location",
			Data:  string(loc),
		}
	}
	client.RCMutex.RUnlock()
}

func createPeerConnection(client *Client) {
	// Configure ICE servers
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.stunprotocol.org"},
			},
		},
	}

	// Create new PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("%+v\n", err)
	}

	peerConnection.OnNegotiationNeeded(func() {
		// log.Printf("PC EVENT: renegotiation needed, %s \n", peerConnection.SignalingState())

		if peerConnection.SignalingState() != webrtc.SignalingStateStable {
			log.Println("Blocked renegotiation due to pending offer")
			return
		}

		offer, err := peerConnection.CreateOffer(nil)
		if err != nil {
			log.Printf("Error renegotiating offer: %s", err)
		}

		if err = client.PeerConnection.SetLocalDescription(offer); err != nil {
			log.Printf("Error setting local description: %s", err)
		}

		off, err := json.Marshal(offer)
		if err != nil {
			log.Printf("Error marshaling renegotiation offer: %s", err)
		}

		client.WriteChan <- &types.WebsocketMessage{
			Event: "wrtc_renegotiation_needed",
			Data:  string(off),
		}
	})

	// Trickle ICE handler
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		candidateString, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			fmt.Printf("%+v\n", err)
			return
		}

		client.WriteChan <- &types.WebsocketMessage{
			Event: "wrtc_candidate",
			Data:  string(candidateString),
		}
	})

	// If the peer connection fails...
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {

		if connectionState == webrtc.ICEConnectionStateFailed {
			client.WriteChan <- &types.WebsocketMessage{
				Event: "wrtc_failed",
			}
			if closeErr := peerConnection.Close(); closeErr != nil {
				log.Printf("Error closing the peer connection %s", closeErr)
			}

			log.Println("Peer connection state failed. Handling client disconnect")
			handleDisconnect(client)
		}
	})

	peerConnection.OnTrack(func(tr *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		buff := make([]byte, 1500)
		for {
			i, _, err := tr.Read(buff)
			if err != nil {
				return
			}
			client.InboundAudio <- buff[:i]
		}
	})

	// Send incoming audio packets to all clients registered to this client.
	go RouteAudioToClients(client)

	client.PCMutex.Lock()
	client.PeerConnection = peerConnection
	client.PCMutex.Unlock()

	// Start locating and connecting to other clients.
	go locateAndConnect(client)
}

func handleRenegotiation(client *Client, message *types.WebsocketMessage) {

	remoteOffer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &remoteOffer); err != nil {
		log.Print(err)
	}

	if client.PeerConnection.SignalingState() != webrtc.SignalingStateStable {
		client.PeerConnection.SetLocalDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeRollback,
		})
		client.PeerConnection.SetRemoteDescription(remoteOffer)
	}

	if err := client.PeerConnection.SetRemoteDescription(remoteOffer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

	answer, err := client.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Printf("Error creating renegotiation answer %s", err)
	}

	if err = client.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Printf("Error setting local description: %s", err)
	}

	ans, _ := json.Marshal(answer)

	client.WriteChan <- &types.WebsocketMessage{
		Event: "wrtc_answer",
		Data:  string(ans),
	}

}

func handleIceCandidate(client *Client, message *types.WebsocketMessage) {
	fmt.Printf("Candidate recvd: %+v", message)
	candidate := webrtc.ICECandidateInit{}
	if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	if err := client.PeerConnection.AddICECandidate(candidate); err != nil {
		fmt.Printf("%+v\n", err)
		return
	}
}

func handleOffer(client *Client, message *types.WebsocketMessage) {

	createPeerConnection(client)

	offer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &offer); err != nil {
		log.Print(err)
	}

	if err := client.PeerConnection.SetRemoteDescription(offer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

	answer, err := client.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Printf("Error creating answer: %s", err)
	}

	if err = client.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Printf("Error setting local description: %s", err)
	}

	ans, _ := json.Marshal(answer)

	client.WriteChan <- &types.WebsocketMessage{
		Event: "wrtc_answer",
		Data:  string(ans),
	}

}

func handleAnswer(client *Client, message *types.WebsocketMessage) {
	answer := webrtc.SessionDescription{}
	if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
		log.Print(err)
	}

	if err := client.PeerConnection.SetRemoteDescription(answer); err != nil {
		log.Printf("Error setting remote description: %s", err)
	}

}

func handleDisconnect(client *Client) {
	// Signal locate and connect goroutine to shutdown
	client.StopLAC <- true

	// Wait until the clients locate and connect goroutine signals that it has stopped
	<-client.StoppedLAC

	client.PCMutex.Lock()

	client.StopRoutingAudio <- true

	// Make a copy of registered clients.
	RegisteredClientsCopy := make(map[uuid.UUID]*Client)

	client.RCMutex.RLock()
	for uuid := range client.RegisteredClients {
		RegisteredClientsCopy[uuid] = client.Nucleus.Clients[uuid]
	}
	client.RCMutex.RUnlock()

	// Unregister all currently registered clients from this one.
	for uuid := range RegisteredClientsCopy {
		client.Nucleus.Mutex.RLock()
		unregister(client.Nucleus.Clients[uuid], client)
		client.Nucleus.Mutex.RUnlock()
	}

	client.PeerConnection.Close()

	client.PeerConnection = nil

	client.PCMutex.Unlock()
}

func shutdownClient(client *Client) {
	log.Printf("Shutting down client %s\n", client.UUID)

	// Unsubscribe this client from the nucleus.
	client.Nucleus.Unsubscribe <- client

	// Wait until the nucleus signals that the client has been removed
	// <-client.RemovedFromNucleus

	client.Socket.Conn.Close()
}
