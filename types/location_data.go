package types

import (
	"math"

	"github.com/google/uuid"
)

var (
	// 1/3 mile in terms of geographical coordinates
	ONE_THIRD_MILE = 0.00483091787
)

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

func WithinRange(from *LocationData, to *LocationData) bool {
	return math.Sqrt(math.Pow((to.Latitude-from.Latitude), 2)+math.Pow((to.Longitude-from.Longitude), 2)) <= ONE_THIRD_MILE
}
