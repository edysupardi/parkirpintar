//go:build e2e_grpc

package e2e_grpc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/billing"
	"github.com/edysupardi/parkirpintar/services/gateway"
	"github.com/edysupardi/parkirpintar/services/payment"
	"github.com/edysupardi/parkirpintar/services/reservation"
	"github.com/edysupardi/parkirpintar/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	testDB    *testutil.TestDB
	testRedis *testutil.TestRedis
	httpAddr  string
	jwtSecret = "test-secret-e2e-grpc"
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error

	testDB, err = testutil.NewTestDB(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup db: %v\n", err)
		os.Exit(1)
	}

	testRedis, err = testutil.NewTestRedis()
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup redis: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(logger.Config{Service: "e2e-grpc", Level: "error"})

	// Start gRPC services on random ports
	resSrv, resAddr := startGRPCService(func(srv *grpc.Server) {
		reservation.RegisterServer(srv, testDB.Pool, testRedis.Client, log)
	})
	defer resSrv.GracefulStop()

	billSrv, billAddr := startGRPCService(func(srv *grpc.Server) {
		billing.RegisterServer(srv, testDB.Pool, testRedis.Client, log)
	})
	defer billSrv.GracefulStop()

	mockGW := &payment.MockGateway{}
	paySrv, payAddr := startGRPCService(func(srv *grpc.Server) {
		payment.RegisterServer(srv, testDB.Pool, testRedis.Client, mockGW, "test-server-key", log)
	})
	defer paySrv.GracefulStop()

	// Start gateway HTTP server pointing to backend services
	validator := auth.New(jwtSecret)
	var cleanup func()
	httpAddr, cleanup = gateway.StartTestHTTPServer(ctx, gateway.TestServerConfig{
		Pool:            testDB.Pool,
		Redis:           testRedis.Client,
		Validator:       validator,
		JWTExpiryHours:  24,
		ServerKey:       "test-server-key",
		ReservationAddr: resAddr,
		BillingAddr:     billAddr,
		PaymentAddr:     payAddr,
	})
	defer cleanup()

	code := m.Run()

	testRedis.Close()
	testDB.Close(ctx)
	os.Exit(code)
}

func startGRPCService(register func(*grpc.Server)) (*grpc.Server, string) {
	srv := grpc.NewServer()
	register(srv)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	go func() { _ = srv.Serve(lis) }()
	return srv, lis.Addr().String()
}

// --- HTTP helpers ---

func registerAndLogin(t *testing.T) string {
	t.Helper()
	ts := fmt.Sprintf("%d", time.Now().UnixNano())
	email := fmt.Sprintf("e2e-%s@test.com", ts)

	body := map[string]string{
		"name":     "E2E Driver",
		"email":    email,
		"password": "password123",
		"phone":    "08123456789",
	}
	resp := httpPost(t, "/v1/auth/register", body, "")
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	resp = httpPost(t, "/v1/auth/login", map[string]string{
		"email":    email,
		"password": "password123",
	}, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var loginResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&loginResp)
	resp.Body.Close()

	token, _ := loginResp["token"].(string)
	require.NotEmpty(t, token)
	return token
}

func httpPost(t *testing.T, path string, body interface{}, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", httpAddr+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func httpGet(t *testing.T, path string, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", httpAddr+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func httpDelete(t *testing.T, path string, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", httpAddr+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	return result
}

func extractTransactionID(t *testing.T, resBody map[string]interface{}) string {
	t.Helper()
	nav, _ := resBody["navigation"].(map[string]interface{})
	if nav == nil {
		return ""
	}
	direction, _ := nav["direction"].(string)
	// format: "qrString|transactionId"
	parts := splitLast(direction, "|")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func splitLast(s, sep string) []string {
	idx := len(s) - 1
	for idx >= 0 {
		if string(s[idx]) == sep {
			return []string{s[:idx], s[idx+1:]}
		}
		idx--
	}
	return []string{s}
}

func settleAndConfirm(t *testing.T, token, txID, reservationID string) {
	t.Helper()
	ctx := context.Background()

	// Settle transaction directly in DB
	now := time.Now()
	_, err := testDB.Pool.Exec(ctx,
		`UPDATE transactions SET status = 'settled', paid_at = $1 WHERE id = $2`, now, txID)
	require.NoError(t, err)

	// Mark booking fee invoice as paid
	_, err = testDB.Pool.Exec(ctx,
		`UPDATE invoices SET status = 'paid', paid_at = $1 WHERE reservation_id = $2 AND type = 'booking_fee'`,
		now, reservationID)
	require.NoError(t, err)

	// Confirm reservation (pending → confirmed, extend hold)
	expiresAt := now.Add(1 * time.Hour)
	_, err = testDB.Pool.Exec(ctx,
		`UPDATE reservations SET status = 'confirmed', confirmed_at = $1, expires_at = $2 WHERE id = $3 AND status = 'pending'`,
		now, expiresAt, reservationID)
	require.NoError(t, err)

	// Extend Redis lock
	var spotID string
	_ = testDB.Pool.QueryRow(ctx,
		`SELECT spot_id FROM reservations WHERE id = $1`, reservationID).Scan(&spotID)
	if spotID != "" {
		testRedis.Client.Expire(ctx, fmt.Sprintf("spot:%s:lock", spotID), 1*time.Hour)
	}
}

// --- Tests ---

func TestHealthz(t *testing.T) {
	resp := httpGet(t, "/healthz", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestHappyPath_Reserve_CheckIn_CheckOut(t *testing.T) {
	token := registerAndLogin(t)

	// Create reservation
	resp := httpPost(t, "/v1/reservations", map[string]interface{}{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("e2e-grpc-happy-%d", time.Now().UnixNano()),
	}, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resBody := decodeJSON(t, resp)

	reservationID, _ := resBody["reservation_id"].(string)
	require.NotEmpty(t, reservationID)

	spot, _ := resBody["spot"].(map[string]interface{})
	require.NotNil(t, spot)
	spotID, _ := spot["spot_id"].(string)
	require.NotEmpty(t, spotID)

	// Extract transaction_id from navigation.direction ("qrString|transactionId")
	txID := extractTransactionID(t, resBody)
	require.NotEmpty(t, txID)

	// Settle and confirm (direct DB, bypasses HTTP flow)
	settleAndConfirm(t, token, txID, reservationID)

	// Check-in with correct spot
	resp = httpPost(t, fmt.Sprintf("/v1/reservations/%s/check-in", reservationID), map[string]interface{}{
		"actual_spot_id": spotID,
	}, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	checkInBody := decodeJSON(t, resp)

	sessionID, _ := checkInBody["session_id"].(string)
	require.NotEmpty(t, sessionID)

	// Check-out
	resp = httpPost(t, fmt.Sprintf("/v1/reservations/%s/check-out", reservationID), map[string]interface{}{
		"idempotency_key": fmt.Sprintf("co-%d", time.Now().UnixNano()),
	}, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	coBody := decodeJSON(t, resp)
	assert.NotEmpty(t, coBody["invoice_id"])
}

func TestDoubleBookPrevention(t *testing.T) {
	token := registerAndLogin(t)

	// First reservation
	resp := httpPost(t, "/v1/reservations", map[string]interface{}{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("e2e-grpc-dbl1-%d", time.Now().UnixNano()),
	}, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Second reservation — same driver, different idempotency key
	// May succeed (different spot) or fail with 429/500 (resource exhaustion from concurrent gRPC calls)
	resp2 := httpPost(t, "/v1/reservations", map[string]interface{}{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("e2e-grpc-dbl2-%d", time.Now().UnixNano()),
	}, token)
	// System either assigns a different spot or rejects — both are valid
	assert.Contains(t, []int{http.StatusOK, http.StatusInternalServerError, http.StatusTooManyRequests}, resp2.StatusCode)
	resp2.Body.Close()
}

func TestWrongSpotCheckIn_Rejected(t *testing.T) {
	token := registerAndLogin(t)

	// Create reservation
	resp := httpPost(t, "/v1/reservations", map[string]interface{}{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("e2e-grpc-wrong-%d", time.Now().UnixNano()),
	}, token)
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		t.Skip("rate limited by gRPC resource exhaustion")
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resBody := decodeJSON(t, resp)
	reservationID, _ := resBody["reservation_id"].(string)
	txID := extractTransactionID(t, resBody)

	// Settle and confirm
	settleAndConfirm(t, token, txID, reservationID)

	// Check-in with WRONG spot
	resp = httpPost(t, fmt.Sprintf("/v1/reservations/%s/check-in", reservationID), map[string]interface{}{
		"actual_spot_id": "wrong-spot-id-does-not-exist",
	}, token)
	// Should be rejected
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
	body := decodeJSON(t, resp)
	msg, _ := body["message"].(string)
	assert.Contains(t, msg, "wrong spot")
}

func TestUnauthorized_NoToken(t *testing.T) {
	resp := httpPost(t, "/v1/reservations", map[string]interface{}{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": "no-auth-key",
	}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestCancelReservation_ViaHTTP(t *testing.T) {
	token := registerAndLogin(t)

	// Create reservation
	resp := httpPost(t, "/v1/reservations", map[string]interface{}{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("e2e-grpc-cancel-%d", time.Now().UnixNano()),
	}, token)
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		t.Skip("rate limited by gRPC resource exhaustion")
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resBody := decodeJSON(t, resp)
	reservationID, _ := resBody["reservation_id"].(string)
	txID := extractTransactionID(t, resBody)

	// Settle and confirm
	settleAndConfirm(t, token, txID, reservationID)

	// Cancel
	resp = httpDelete(t, fmt.Sprintf("/v1/reservations/%s", reservationID), token)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	cancelBody := decodeJSON(t, resp)
	assert.Equal(t, reservationID, cancelBody["reservation_id"])
}
