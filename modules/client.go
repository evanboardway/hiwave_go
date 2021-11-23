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

	// A channel whose data is written to the websocket.
	WriteChan chan *types.WebsocketMessage

	// A channel for registering clients to their audio stream.
	Register chan *Client

	// A channel for unregistering clients to their audio stream.
	Unregister chan *Client

	// A track referencing audio packets being sent from the client.
	InboundAudio chan []byte

	// A channel to stop the registration goroutine
	StopRegistration chan bool

	// A channel to stop routing audio to peers
	StopRoutingAudio chan bool

	// A channel to stop locate and connect function
	StopLAC chan bool

	// A map of client uuid (key) to outbound audio tracks (value).
	RegisteredClients map[uuid.UUID]*types.AudioBundle

	// List of clients awaiting registration
	RegistrationQueue *[]uuid.UUID

	// A reference to the peers peer connection object.
	PeerConnection *webrtc.PeerConnection

	// The clients current location
	CurrentLocation *types.LocationData

	// A mutex to lock a client so that only one resource can modify its peer connection at a time.
	PCMutex sync.Mutex

	// A mutex to lock the registered clients list
	RCMutex sync.RWMutex

	// A mutex to lock the registration process
	RegistrationQueueMutex sync.Mutex
}

func NewClient(safeConn *types.ThreadSafeWriter, nucleus *Nucleus) *Client {
	log.Printf("New client")

	return &Client{
		UUID:              uuid.New(),
		Nucleus:           nucleus,
		Socket:            safeConn,
		WriteChan:         make(chan *types.WebsocketMessage),
		Register:          make(chan *Client),
		Unregister:        make(chan *Client),
		StopRegistration:  make(chan bool),
		StopRoutingAudio:  make(chan bool),
		StopLAC:           make(chan bool),
		RegisteredClients: make(map[uuid.UUID]*types.AudioBundle),
		RegistrationQueue: new([]uuid.UUID),
		InboundAudio:      make(chan []byte, 1500),
	}
}

func Reader(client *Client) {

	// If the loop ever breaks (can no longer read from the client socket)
	// we remove the client from the nucleus and close the socket.
	defer func() {
		// peer connection close
		client.Nucleus.Unsubscribe <- client
		client.Socket.Conn.Close()
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

		// if message.Event != "update_location" {
		// 	log.Printf("New message from %s: %+v", client.UUID, message.Event)
		// }

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
			for _, member := range client.Nucleus.Clients {
				member.Register <- client
			}
			break

		case "mute":
			for _, member := range client.Nucleus.Clients {
				member.Unregister <- client
			}
			// log.Printf("TRIGGERED LISTING: %+v\n", client.RegisteredClients)
			// client.RegisteredClients[client.UUID] = &types.AudioBundle{}

			// bundle := client.RegisteredClients[client.UUID]

			// log.Printf("%+v\n", bundle)

			// client.WriteChan <- &types.WebsocketMessage{
			// 	Event: "test",
			// 	Data:  "testing",
			// }
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
		client.Socket.Conn.Close()
		client.Nucleus.Unsubscribe <- client
	}()

	for {
		data := <-client.WriteChan
		// log.Printf("Writing to client, %+v", data.Event)
		err := client.Socket.Conn.WriteJSON(data)
		if err != nil {
			// if writing to the socket fails, function returns and defer block is called
			log.Printf("Write error %+v", err)
			return
		}
	}
}

func Registration(client *Client) {
	for {
		// select statement in golang is nonblocking to the channel.
		select {
		case registree := <-client.Register:

			client.RCMutex.Lock()
			fmt.Printf("%s REG Locked RC MUTEX\n", client.UUID)

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

			bundle := &types.AudioBundle{
				Transceiver: transceiver,
				Track:       newTrack,
			}

			client.RegisteredClients[registree.UUID] = bundle
			log.Printf("Registered client %s to client %s\n", registree.UUID, client.UUID)
			client.RCMutex.Unlock()
			fmt.Printf("%s REG Unlocked RC MUTEX\n", client.UUID)
			break
		case unregistree := <-client.Unregister:

			log.Printf("Unregistering client %s from client %s\n", unregistree.UUID, client.UUID)
			client.RCMutex.Lock()

			// Peer connection needs to be in a stable state, lock the client mutex.
			unregistreeBundle := client.RegisteredClients[unregistree.UUID]
			log.Printf("Unregistree audio bundle: %+v cli: %s\n", unregistreeBundle, client.UUID)

			if err := unregistree.PeerConnection.RemoveTrack(unregistreeBundle.Transceiver.Sender()); err != nil {
				log.Printf("Error removing track on unregistree peer connection %s\n", err)
			}

			unregistreeBundle.Transceiver.Stop()

			delete(client.RegisteredClients, unregistree.UUID)
			log.Printf("Unregistered client %s from client %s\n", unregistree.UUID, client.UUID)

			client.RCMutex.Unlock()

			client.WriteChan <- &types.WebsocketMessage{
				Event: "wrtc_remove_track",
				Data:  unregistree.UUID.String(),
			}
			break
		case <-client.StopRegistration:
			return
		}
	}
}

// Register and unregister peers to this clients's audio streams based on relative location.
func locateAndConnect(client *Client) {
	for {
		select {
		case <-client.StopLAC:
			return
		default:
			// Compile a list of all the clients that are connected and not the same as the client param.
			client.Nucleus.Mutex.RLock()
			filtered_clients := make(map[uuid.UUID]*Client)
			for _, peer := range client.Nucleus.Clients {
				if peer.PeerConnection != nil && peer.UUID != client.UUID {
					filtered_clients[peer.UUID] = peer
				}
			}
			client.Nucleus.Mutex.RUnlock()

			for peer_uuid, peer := range filtered_clients {
				within_range := types.WithinRange(client.CurrentLocation, peer.CurrentLocation)
				client.RCMutex.RLock()
				fmt.Printf("%s LAC Locked RC Mut\n", client.UUID)
				registered := client.RegisteredClients[peer_uuid]
				if registered != nil {
					if within_range == false {
						client.WriteChan <- &types.WebsocketMessage{
							Event: "peer",
							Data:  "disconnected peer" + peer.UUID.String(),
						}
						fmt.Printf("%s LAC Unregister peer\n", client.UUID)
						client.Unregister <- peer
					} else {
						fmt.Printf("%s LAC registered and in range\n", client.UUID)
					}
				} else if within_range {
					client.WriteChan <- &types.WebsocketMessage{
						Event: "peer",
						Data:  "connected peer" + peer.UUID.String(),
					}
					fmt.Printf("%s LAC register peer %s\n", client.UUID, peer_uuid)
					client.Register <- peer
				} else {
					fmt.Printf("%s LAC noop\n", client.UUID)
				}
				fmt.Printf("%s LAC Unlocked RC Mut\n", client.UUID)
				client.RCMutex.RUnlock()
			}
		}
	}
}

// locateAndConnect attempts to unregister a client while they're in the process of unregistering already

func RouteAudioToClients(client *Client) {
	for {
		select {
		case packet := <-client.InboundAudio:
			for _, registreeBundle := range client.RegisteredClients {
				registreeBundle.Track.Write(packet)
			}
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

	// log.Printf("%+v\n", location)
	client.CurrentLocation = location

	// loc, err := json.Marshal(bundle)
	// if err != nil {
	// 	log.Printf("Error marshaling client location: %s", err)
	// }
	// client.WriteChan <- &types.WebsocketMessage{
	// 	Event: "peer_location",
	// 	Data:  string(loc),
	// }

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
		// log.Printf("Connection State has changed to %s \n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			client.WriteChan <- &types.WebsocketMessage{
				Event: "wrtc_failed",
			}
			if closeErr := peerConnection.Close(); closeErr != nil {
				log.Printf("Error closing the peer connection %s", closeErr)
			}

			client.PeerConnection = nil
		}
	})

	peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Println("DATA CHAN")
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
	fmt.Printf("%s STARTING LAC\n", client.UUID)
	go locateAndConnect(client)
}

func handleDisconnect(client *Client) {
	if client.PeerConnection != nil {
		client.StopRoutingAudio <- true
		client.PeerConnection.Close()
		client.PeerConnection = nil
	}
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

func shutdownClient(client *Client) {
	client.StopRegistration <- true
	client.StopRoutingAudio <- true
	client.StopLAC <- true
}
