package pricing

import "time"

const (
	BookingFee        int64 = 5_000
	HourlyRate        int64 = 5_000
	OvernightFee      int64 = 20_000
	WrongSpotPenalty  int64 = 200_000
	CancellationFree  int64 = 0
	CancellationFee   int64 = 5_000
	NoShowFee         int64 = 10_000
	FreeCancelWindow        = 2 * time.Minute
)

// Result holds the complete billing breakdown
type Result struct {
	BookingFee    int64
	ParkingFee    int64
	OvernightFee  int64
	PenaltyFee    int64
	TotalAmount   int64
	BilledHours   int32
	IsOvernight   bool
}

// Calculate computes the full billing for a parking session.
// checkIn and checkOut are in UTC — caller responsible for timezone.
func Calculate(checkIn, checkOut time.Time, wrongSpot bool) Result {
	// TODO: implement
	return Result{}
}

// CalculateCancellationFee returns the fee based on when driver cancels.
func CalculateCancellationFee(confirmedAt, cancelledAt time.Time, isNoShow bool) int64 {
	// TODO: implement
	return 0
}

// IsOvernight returns true if the session crosses midnight WIB (UTC+7).
func IsOvernight(checkIn, checkOut time.Time) bool {
	// TODO: implement
	return false
}

// BilledHours returns the ceiling of actual duration in hours.
func BilledHours(checkIn, checkOut time.Time) int32 {
	// TODO: implement
	return 0
}
