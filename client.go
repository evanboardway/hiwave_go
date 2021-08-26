package main

import (
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type Client struct {
	// Websocket connection to communicate
	conn *websocket.Conn

	// A place to store our incoming audio track
	audioTrack *webrtc.TrackLocalStaticRTP

	// Peer connection object
	peerConnection *webrtc.PeerConnection
}

func NewClient(connection *websocket.Conn, track *webrtc.TrackLocalStaticRTP, peer *webrtc.PeerConnection) *Client {
	return &Client{
		conn:           connection,
		audioTrack:     track,
		peerConnection: peer,
	}
}
