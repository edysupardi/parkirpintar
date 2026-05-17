package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("ACCEPTANCE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	os.Exit(m.Run())
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

func register(t *testing.T) (driverID, token string) {
	t.Helper()
	email := fmt.Sprintf("accept-%d@test.com", time.Now().UnixNano())
	body, _ := json.Marshal(map[string]string{
		"name": "Acceptance", "email": email, "phone": "08100000001",
		"password": "pass123", "vehicle_type": "car",
	})
	resp, err := httpClient().Post(baseURL+"/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result["driver_id"].(string), result["token"].(string)
}

func authReq(t *testing.T, method, path string, body any, token string) *http.Response {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, baseURL+path, reqBody)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

// ── Acceptance Test: Full Happy Path ────────────────────────────────────────

func TestAcceptance_HappyPath_ReserveToPayment(t *testing.T) {
	_, token := register(t)

	// 1. Check availability
	resp := authReq(t, "GET", "/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR", nil, token)
	avail := decodeJSON(t, resp)
	assert.Equal(t, true, avail["is_available"])
	totalBefore := avail["available_spots"].(float64)

	// 2. Create reservation (returns pending + QRIS payment)
	resp = authReq(t, "POST", "/v1/reservations", map[string]string{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("accept-%d", time.Now().UnixNano()),
	}, token)
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skip("rate limited")
	}
	reservation := decodeJSON(t, resp)
	reservationID := reservation["reservation_id"].(string)
	assert.NotEmpty(t, reservationID)
	assert.NotNil(t, reservation["spot"])
	assert.NotNil(t, reservation["navigation"])

	// Extract transaction ID from navigation.direction
	nav := reservation["navigation"].(map[string]any)
	direction := nav["direction"].(string)
	// direction format: "https://...tref=...|<transaction_id>"
	var txID string
	for i := len(direction) - 1; i >= 0; i-- {
		if direction[i] == '|' {
			txID = direction[i+1:]
			break
		}
	}
	require.NotEmpty(t, txID, "transaction_id should be in navigation.direction")

	// 3. Simulate payment settle
	resp = authReq(t, "POST", "/v1/payments/simulate-settle", map[string]string{
		"transaction_id": txID,
	}, token)
	settle := decodeJSON(t, resp)
	assert.Equal(t, "settled", settle["status"])

	// 4. Confirm booking payment
	resp = authReq(t, "POST", "/v1/reservations/confirm-payment", map[string]string{
		"transaction_id": txID,
		"reservation_id": reservationID,
	}, token)
	confirm := decodeJSON(t, resp)
	assert.Equal(t, "confirmed", confirm["status"])

	// 5. Check-in
	resp = authReq(t, "POST", fmt.Sprintf("/v1/reservations/%s/check-in", reservationID), map[string]string{}, token)
	checkIn := decodeJSON(t, resp)
	assert.NotEmpty(t, checkIn["session_id"])

	// 6. Check-out
	resp = authReq(t, "POST", fmt.Sprintf("/v1/reservations/%s/check-out", reservationID), map[string]string{
		"idempotency_key": fmt.Sprintf("co-%d", time.Now().UnixNano()),
	}, token)
	checkOut := decodeJSON(t, resp)
	assert.NotEmpty(t, checkOut["invoice_id"])

	// 7. Verify availability restored
	resp = authReq(t, "GET", "/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR", nil, token)
	availAfter := decodeJSON(t, resp)
	assert.Equal(t, totalBefore, availAfter["available_spots"].(float64))
}

// ── Acceptance Test: Reservation Cancellation ───────────────────────────────

func TestAcceptance_CancelReservation(t *testing.T) {
	_, token := register(t)

	// Create reservation (use motorcycle to avoid spot contention with other tests)
	resp := authReq(t, "POST", "/v1/reservations", map[string]string{
		"vehicle_type":    "VEHICLE_TYPE_MOTORCYCLE",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("accept-cancel-%d", time.Now().UnixNano()),
	}, token)
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skip("rate limited")
	}
	reservation := decodeJSON(t, resp)
	reservationID, ok := reservation["reservation_id"].(string)
	if !ok || reservationID == "" {
		t.Skipf("no spots available — response: %v", reservation)
	}

	// Cancel
	resp = authReq(t, "DELETE", fmt.Sprintf("/v1/reservations/%s", reservationID), nil, token)
	cancel := decodeJSON(t, resp)
	cancelStatus, _ := cancel["status"].(string)
	assert.Contains(t, cancelStatus, "CANCELLED")
}

// ── Acceptance Test: Double Booking Prevention ──────────────────────────────

func TestAcceptance_DoubleBookPrevention(t *testing.T) {
	_, token1 := register(t)
	_, token2 := register(t)

	// Driver 1 reserves
	resp := authReq(t, "POST", "/v1/reservations", map[string]string{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("accept-db1-%d", time.Now().UnixNano()),
	}, token1)
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skip("rate limited")
	}
	res1 := decodeJSON(t, resp)
	spot := res1["spot"].(map[string]any)
	spotID := spot["spot_id"].(string)

	// Driver 2 tries same spot
	resp = authReq(t, "POST", "/v1/reservations", map[string]string{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_USER_SELECTED",
		"spot_id":         spotID,
		"idempotency_key": fmt.Sprintf("accept-db2-%d", time.Now().UnixNano()),
	}, token2)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// ── Acceptance Test: Unauthorized Access ────────────────────────────────────

func TestAcceptance_UnauthorizedAccess(t *testing.T) {
	// No token
	req, _ := http.NewRequest("GET", baseURL+"/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR", nil)
	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Invalid token
	req, _ = http.NewRequest("GET", baseURL+"/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err = httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ── Acceptance Test: Reservation History ────────────────────────────────────

func TestAcceptance_ReservationHistory(t *testing.T) {
	_, token := register(t)

	// Create and cancel a reservation to have history
	resp := authReq(t, "POST", "/v1/reservations", map[string]string{
		"vehicle_type":    "VEHICLE_TYPE_MOTORCYCLE",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("accept-hist-%d", time.Now().UnixNano()),
	}, token)
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skip("rate limited")
	}
	res := decodeJSON(t, resp)
	reservationID := res["reservation_id"].(string)

	authReq(t, "DELETE", fmt.Sprintf("/v1/reservations/%s", reservationID), nil, token)

	// Get history
	resp = authReq(t, "GET", "/v1/reservations/history", nil, token)
	history := decodeJSON(t, resp)
	reservations, ok := history["reservations"].([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(reservations), 1)
}
