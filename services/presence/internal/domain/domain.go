package domain

import (
	"context"
	"math"
	"time"
)

const GeofenceRadius = 50.0 // meters

type LocationUpdate struct {
	DriverID      string
	SessionID     string
	ReservationID string
	Latitude      float64
	Longitude     float64
	Accuracy      float32
	RecordedAt    time.Time
}

type Repository interface {
	SaveLocation(ctx context.Context, update LocationUpdate) error
	GetLastLocation(ctx context.Context, driverID string) (*LocationUpdate, error)
}

// HaversineDistance returns distance in meters between two coordinates.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}
