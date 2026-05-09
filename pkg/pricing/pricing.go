package pricing

import "time"

const (
	BookingFee       int64 = 5_000
	HourlyRate       int64 = 5_000
	OvernightFee     int64 = 20_000
	CancellationFree int64 = 0
	CancellationFee  int64 = 5_000
	NoShowFee        int64 = 0 // booking fee 5,000 already charged at confirmation
	FreeCancelWindow       = 2 * time.Minute
)

// Result holds the complete billing breakdown
type Result struct {
	BookingFee   int64
	ParkingFee   int64
	OvernightFee int64
	TotalAmount  int64
	BilledHours  int32
	IsOvernight  bool
}

// Calculate computes the billing for a parking session (excludes booking fee — charged separately at reservation).
func Calculate(checkIn, checkOut time.Time) Result {
	billed := BilledHours(checkIn, checkOut)
	overnight := IsOvernight(checkIn, checkOut)

	parkingFee := int64(billed) * HourlyRate
	overnightFee := int64(0)
	if overnight {
		overnightFee = OvernightFee
	}

	return Result{
		BookingFee:   BookingFee,
		ParkingFee:   parkingFee,
		OvernightFee: overnightFee,
		TotalAmount:  parkingFee + overnightFee,
		BilledHours:  billed,
		IsOvernight:  overnight,
	}
}

// CalculateCancellationFee returns the fee based on when driver cancels.
func CalculateCancellationFee(confirmedAt, cancelledAt time.Time, isNoShow bool) int64 {
	if isNoShow {
		return NoShowFee
	}
	if cancelledAt.Sub(confirmedAt) <= FreeCancelWindow {
		return CancellationFree
	}
	return CancellationFee
}

// IsOvernight returns true if the session crosses midnight WIB (UTC+7).
func IsOvernight(checkIn, checkOut time.Time) bool {
	wib := time.FixedZone("WIB", 7*60*60)
	inWIB := checkIn.In(wib)
	outWIB := checkOut.In(wib)
	inDate := time.Date(inWIB.Year(), inWIB.Month(), inWIB.Day(), 0, 0, 0, 0, wib)
	outDate := time.Date(outWIB.Year(), outWIB.Month(), outWIB.Day(), 0, 0, 0, 0, wib)
	return outDate.After(inDate)
}

// BilledHours returns the ceiling of actual duration in hours.
func BilledHours(checkIn, checkOut time.Time) int32 {
	mins := int64(checkOut.Sub(checkIn).Minutes())
	if mins <= 0 {
		return 0
	}
	return int32((mins + 59) / 60)
}
