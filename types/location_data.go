package types

import "github.com/google/uuid"

type LocationData struct {
	Altitude         float64
	AltitudeAccuracy float64
	Latitude         float64
	Accuracy         float64
	Longitude        float64
	Heading          float64
	Speed            float64
}

type LocationBundle struct {
	UUID     uuid.UUID
	Location *LocationData
}
