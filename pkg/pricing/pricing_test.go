package pricing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/org/parkirpintar/pkg/pricing"
)

// TODO: implement all test cases
// These are the skeleton — fill in as you implement pricing.go

func TestBookingFee(t *testing.T) {
	assert.Equal(t, int64(5_000), pricing.BookingFee)
}

func TestBilledHours_ExactOneHour(t *testing.T) {
	checkIn  := time.Now()
	checkOut := checkIn.Add(1 * time.Hour)
	assert.Equal(t, int32(1), pricing.BilledHours(checkIn, checkOut))
}

func TestBilledHours_StartedHour(t *testing.T) {
	checkIn  := time.Now()
	checkOut := checkIn.Add(1*time.Hour + 1*time.Minute)
	assert.Equal(t, int32(2), pricing.BilledHours(checkIn, checkOut))
}

func TestIsOvernight_CrossingMidnight(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	checkIn  := time.Date(2025, 1, 1, 23, 0, 0, 0, wib)
	checkOut := time.Date(2025, 1, 2, 1, 0, 0, 0, wib)
	assert.True(t, pricing.IsOvernight(checkIn, checkOut))
}

func TestIsOvernight_SameDay(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	checkIn  := time.Date(2025, 1, 1, 10, 0, 0, 0, wib)
	checkOut := time.Date(2025, 1, 1, 12, 0, 0, 0, wib)
	assert.False(t, pricing.IsOvernight(checkIn, checkOut))
}

func TestCalculate_NormalSession(t *testing.T) {
	checkIn  := time.Now()
	checkOut := checkIn.Add(2 * time.Hour)
	result   := pricing.Calculate(checkIn, checkOut, false)
	assert.Equal(t, int64(10_000), result.ParkingFee)
	assert.Equal(t, int64(5_000),  result.BookingFee)
	assert.Equal(t, int64(0),      result.OvernightFee)
	assert.Equal(t, int64(0),      result.PenaltyFee)
	assert.Equal(t, int64(15_000), result.TotalAmount)
}

func TestCalculate_WrongSpot(t *testing.T) {
	checkIn  := time.Now()
	checkOut := checkIn.Add(1 * time.Hour)
	result   := pricing.Calculate(checkIn, checkOut, true)
	assert.Equal(t, int64(200_000), result.PenaltyFee)
}

func TestCalculateCancellationFee_Under2Min(t *testing.T) {
	confirmed := time.Now()
	cancelled := confirmed.Add(1 * time.Minute)
	assert.Equal(t, int64(0), pricing.CalculateCancellationFee(confirmed, cancelled, false))
}

func TestCalculateCancellationFee_Over2Min(t *testing.T) {
	confirmed := time.Now()
	cancelled := confirmed.Add(5 * time.Minute)
	assert.Equal(t, int64(5_000), pricing.CalculateCancellationFee(confirmed, cancelled, false))
}

func TestCalculateCancellationFee_NoShow(t *testing.T) {
	confirmed := time.Now()
	cancelled := confirmed.Add(61 * time.Minute)
	assert.Equal(t, int64(10_000), pricing.CalculateCancellationFee(confirmed, cancelled, true))
}
