package smoke_test

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
	baseURL = os.Getenv("SMOKE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	os.Exit(m.Run())
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func TestSmoke_Healthz(t *testing.T) {
	resp, err := httpClient().Get(baseURL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSmoke_Register(t *testing.T) {
	email := fmt.Sprintf("smoke-%d@test.com", time.Now().UnixNano())
	body, _ := json.Marshal(map[string]string{
		"name":         "Smoke Test",
		"email":        email,
		"phone":        "08123456789",
		"password":     "smoketest123",
		"vehicle_type": "car",
	})

	resp, err := httpClient().Post(baseURL+"/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.NotEmpty(t, result["token"])
	assert.NotEmpty(t, result["driver_id"])
}

func TestSmoke_Login(t *testing.T) {
	email := fmt.Sprintf("smoke-login-%d@test.com", time.Now().UnixNano())

	// register first
	regBody, _ := json.Marshal(map[string]string{
		"name": "Smoke Login", "email": email, "phone": "08100000000",
		"password": "pass123", "vehicle_type": "car",
	})
	regResp, err := httpClient().Post(baseURL+"/v1/auth/register", "application/json", bytes.NewReader(regBody))
	require.NoError(t, err)
	regResp.Body.Close()

	// login
	loginBody, _ := json.Marshal(map[string]string{"email": email, "password": "pass123"})
	resp, err := httpClient().Post(baseURL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.NotEmpty(t, result["token"])
}

func TestSmoke_Availability(t *testing.T) {
	token := registerAndGetToken(t)

	req, _ := http.NewRequest("GET", baseURL+"/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.NotNil(t, result["total_capacity"])
}

func TestSmoke_CreateReservation(t *testing.T) {
	token := registerAndGetToken(t)

	body, _ := json.Marshal(map[string]string{
		"vehicle_type":    "VEHICLE_TYPE_CAR",
		"assignment_mode": "ASSIGNMENT_MODE_SYSTEM",
		"idempotency_key": fmt.Sprintf("smoke-%d", time.Now().UnixNano()),
	})

	req, _ := http.NewRequest("POST", baseURL+"/v1/reservations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 429 = rate limited, acceptable in smoke test (service is alive)
	if resp.StatusCode == http.StatusTooManyRequests {
		t.Skip("rate limited — service is alive but throttled")
	}

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.NotEmpty(t, result["reservation_id"])
	assert.NotNil(t, result["spot"])
}

func TestSmoke_Unauthorized(t *testing.T) {
	req, _ := http.NewRequest("GET", baseURL+"/v1/parking/availability?vehicle_type=VEHICLE_TYPE_CAR", nil)
	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func registerAndGetToken(t *testing.T) string {
	t.Helper()
	email := fmt.Sprintf("smoke-%d@test.com", time.Now().UnixNano())
	body, _ := json.Marshal(map[string]string{
		"name": "Smoke", "email": email, "phone": "08100000001",
		"password": "pass123", "vehicle_type": "car",
	})
	resp, err := httpClient().Post(baseURL+"/v1/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result["token"].(string)
}
