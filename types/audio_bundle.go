package types

import "github.com/pion/webrtc/v3"

type AudioBundle struct {
	Transceiver *webrtc.RTPTransceiver
	Track       *webrtc.TrackLocalStaticRTP
}
