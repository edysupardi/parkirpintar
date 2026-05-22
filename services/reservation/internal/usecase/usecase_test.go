package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/usecase"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockRepo struct {
	reservation       *domain.Reservation
	reservations      []domain.Reservation
	spot              *domain.Spot
	spots             []domain.Spot
	available         int32
	insertErr         error
	getErr            error
	updateErr         error
	findSpotErr       error
	countErr          error
	listErr           error
	expiredErr        error
	idempotencyResult *domain.Reservation
}

func (m *mockRepo) InsertReservation(_ context.Context, _ domain.Reservation) error {
	return m.insertErr
}
func (m *mockRepo) GetReservation(_ context.Context, _ string) (*domain.Reservation, error) {
	return m.reservation, m.getErr
}
func (m *mockRepo) GetActiveReservation(_ context.Context, _ string) (*domain.Reservation, error) {
	return m.reservation, m.getErr
}
func (m *mockRepo) GetByIdempotencyKey(_ context.Context, _ string) (*domain.Reservation, error) {
	return m.idempotencyResult, m.getErr
}
func (m *mockRepo) UpdateStatus(_ context.Context, _ string, _ domain.ReservationStatus) error {
	return m.updateErr
}
func (m *mockRepo) UpdateConfirmed(_ context.Context, _ string, _ time.Time, _ time.Time) error {
	return m.updateErr
}
func (m *mockRepo) UpdateCheckIn(_ context.Context, _, _ string, _ time.Time) error {
	return m.updateErr
}
func (m *mockRepo) UpdateCheckOut(_ context.Context, _ string, _ time.Time) error {
	return m.updateErr
}
func (m *mockRepo) UpdateCancelled(_ context.Context, _ string, _ time.Time) error {
	return m.updateErr
}
func (m *mockRepo) FindAvailableSpot(_ context.Context, _ domain.VehicleType) (*domain.Spot, error) {
	return m.spot, m.findSpotErr
}
func (m *mockRepo) GetSpot(_ context.Context, _ string) (*domain.Spot, error) {
	return m.spot, m.findSpotErr
}
func (m *mockRepo) CountAvailable(_ context.Context, _ domain.VehicleType) (int32, error) {
	return m.available, m.countErr
}
func (m *mockRepo) ListAvailableSpots(_ context.Context, _ domain.VehicleType, _ int32) ([]domain.Spot, error) {
	return m.spots, m.listErr
}
func (m *mockRepo) ListExpiredReservations(_ context.Context, _ time.Time) ([]domain.Reservation, error) {
	return m.reservations, m.expiredErr
}

type mockLocker struct {
	acquired bool
	err      error
}

func (m *mockLocker) Acquire(_ context.Context, _, _ string, _ time.Duration) (bool, error) {
	return m.acquired, m.err
}
func (m *mockLocker) Release(_ context.Context, _, _ string) error { return nil }

type mockPublisher struct {
	called int
	err    error
}

func (m *mockPublisher) PublishReservationConfirmed(_ context.Context, _ domain.Reservation) error {
	m.called++
	return m.err
}
func (m *mockPublisher) PublishReservationExpired(_ context.Context, _ domain.Reservation) error {
	m.called++
	return m.err
}
func (m *mockPublisher) PublishReservationCancelled(_ context.Context, _ domain.Reservation) error {
	m.called++
	return m.err
}
func (m *mockPublisher) PublishCheckInDetected(_ context.Context, _ domain.Reservation) error {
	m.called++
	return m.err
}
func (m *mockPublisher) PublishCheckOutCompleted(_ context.Context, _ domain.Reservation) error {
	m.called++
	return m.err
}

// --- Helpers ---

func newTestUsecase(repo domain.Repository, locker domain.Locker, pub domain.EventPublisher, rdb *redis.Client) *usecase.ReservationUsecase {
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	store := idempotency.New(rdb)
	return usecase.New(repo, locker, pub, store, log)
}

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func testSpot() *domain.Spot {
	return &domain.Spot{SpotID: "spot-1", Floor: 1, SpotNumber: 1, VehicleType: domain.VehicleTypeCar}
}

func confirmedReservation() *domain.Reservation {
	now := time.Now()
	return &domain.Reservation{
		ReservationID: "res-1",
		DriverID:      "drv-1",
		Spot:          *testSpot(),
		Status:        domain.StatusConfirmed,
		ConfirmedAt:   now,
		ExpiresAt:     now.Add(domain.HoldDuration),
	}
}

func activeReservation() *domain.Reservation {
	now := time.Now()
	checkIn := now.Add(-30 * time.Minute)
	return &domain.Reservation{
		ReservationID: "res-1",
		DriverID:      "drv-1",
		Spot:          *testSpot(),
		Status:        domain.StatusActive,
		ConfirmedAt:   now.Add(-1 * time.Hour),
		ExpiresAt:     now.Add(1 * time.Hour),
		CheckInAt:     &checkIn,
		SessionID:     "sess-1",
	}
}

// --- CreateReservation Tests ---

func TestCreateReservation_HappyPath_SystemAssigned(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{spot: testSpot()}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	r, err := uc.CreateReservation(context.Background(), "drv-1", "key-1", domain.VehicleTypeCar, domain.AssignmentModeSystem, "")

	require.NoError(t, err)
	assert.Equal(t, "drv-1", r.DriverID)
	assert.Equal(t, domain.StatusPending, r.Status)
	assert.Equal(t, "spot-1", r.Spot.SpotID)
}

func TestCreateReservation_HappyPath_UserSelected(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{spot: testSpot()}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	r, err := uc.CreateReservation(context.Background(), "drv-1", "key-2", domain.VehicleTypeCar, domain.AssignmentModeSelected, "spot-1")

	require.NoError(t, err)
	assert.Equal(t, domain.AssignmentModeSelected, r.AssignmentMode)
}

func TestCreateReservation_IdempotencyHit(t *testing.T) {
	rdb := setupRedis(t)
	cached := &domain.Reservation{ReservationID: "res-cached", DriverID: "drv-1"}
	repo := &mockRepo{spot: testSpot(), idempotencyResult: cached}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)

	// first call
	_, err := uc.CreateReservation(context.Background(), "drv-1", "dup-key", domain.VehicleTypeCar, domain.AssignmentModeSystem, "")
	require.NoError(t, err)

	// second call with same key — should return cached
	r, err := uc.CreateReservation(context.Background(), "drv-1", "dup-key", domain.VehicleTypeCar, domain.AssignmentModeSystem, "")
	require.NoError(t, err)
	assert.Equal(t, "res-cached", r.ReservationID)
}

func TestCreateReservation_LockFail(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{spot: testSpot()}
	locker := &mockLocker{acquired: false}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CreateReservation(context.Background(), "drv-1", "key-3", domain.VehicleTypeCar, domain.AssignmentModeSystem, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "spot unavailable")
}

func TestCreateReservation_NoSpotAvailable(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{spot: nil}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CreateReservation(context.Background(), "drv-1", "key-4", domain.VehicleTypeCar, domain.AssignmentModeSystem, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no available spot")
}

func TestCreateReservation_DBInsertFail_ReleasesLock(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{spot: testSpot(), insertErr: errors.New("db down")}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CreateReservation(context.Background(), "drv-1", "key-5", domain.VehicleTypeCar, domain.AssignmentModeSystem, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert reservation")
}

// --- ConfirmReservation Tests ---

func TestConfirmReservation_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	pending := &domain.Reservation{
		ReservationID: "res-1",
		DriverID:      "drv-1",
		Spot:          *testSpot(),
		Status:        domain.StatusPending,
	}
	repo := &mockRepo{reservation: pending}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	r, err := uc.ConfirmReservation(context.Background(), "res-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusConfirmed, r.Status)
	assert.Equal(t, 1, pub.called)
}

func TestConfirmReservation_WrongStatus(t *testing.T) {
	rdb := setupRedis(t)
	active := activeReservation()
	repo := &mockRepo{reservation: active}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.ConfirmReservation(context.Background(), "res-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot confirm")
}

func TestConfirmReservation_NotFound(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{getErr: errors.New("not found")}
	locker := &mockLocker{acquired: true}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.ConfirmReservation(context.Background(), "res-999")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get reservation")
}

// --- CancelReservation Tests ---

func TestCancelReservation_Under2Min_NoFee(t *testing.T) {
	rdb := setupRedis(t)
	r := &domain.Reservation{
		ReservationID: "res-1",
		DriverID:      "drv-1",
		Spot:          *testSpot(),
		Status:        domain.StatusConfirmed,
		ConfirmedAt:   time.Now().Add(-1 * time.Minute),
	}
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	result, fee, err := uc.CancelReservation(context.Background(), "res-1", "drv-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, result.Status)
	assert.Equal(t, int64(0), fee)
	assert.Equal(t, 1, pub.called)
}

func TestCancelReservation_Over2Min_Fee5000(t *testing.T) {
	rdb := setupRedis(t)
	r := &domain.Reservation{
		ReservationID: "res-1",
		DriverID:      "drv-1",
		Spot:          *testSpot(),
		Status:        domain.StatusConfirmed,
		ConfirmedAt:   time.Now().Add(-5 * time.Minute),
	}
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, fee, err := uc.CancelReservation(context.Background(), "res-1", "drv-1")

	require.NoError(t, err)
	assert.Equal(t, int64(5_000), fee)
}

func TestCancelReservation_WrongDriver(t *testing.T) {
	rdb := setupRedis(t)
	r := confirmedReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, _, err := uc.CancelReservation(context.Background(), "res-1", "drv-other")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by driver")
}

func TestCancelReservation_InvalidStatus(t *testing.T) {
	rdb := setupRedis(t)
	r := activeReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, _, err := uc.CancelReservation(context.Background(), "res-1", "drv-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be cancelled")
}

// --- CheckIn Tests ---

func TestCheckIn_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	r := confirmedReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	result, err := uc.CheckIn(context.Background(), "res-1", "drv-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusActive, result.Status)
	assert.NotEmpty(t, result.SessionID)
	assert.Equal(t, 1, pub.called)
}

func TestCheckIn_WrongDriver(t *testing.T) {
	rdb := setupRedis(t)
	r := confirmedReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CheckIn(context.Background(), "res-1", "drv-other")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by driver")
}

func TestCheckIn_Expired(t *testing.T) {
	rdb := setupRedis(t)
	r := confirmedReservation()
	r.ExpiresAt = time.Now().Add(-1 * time.Minute)
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CheckIn(context.Background(), "res-1", "drv-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestCheckIn_WrongStatus(t *testing.T) {
	rdb := setupRedis(t)
	r := activeReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CheckIn(context.Background(), "res-1", "drv-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot check in")
}

// --- CheckOut Tests ---

func TestCheckOut_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	r := activeReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	result, err := uc.CheckOut(context.Background(), "res-1", "drv-1", "co-key-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusCompleted, result.Status)
	assert.NotNil(t, result.CheckOutAt)
	assert.Equal(t, 1, pub.called)
}

func TestCheckOut_IdempotencyHit(t *testing.T) {
	rdb := setupRedis(t)
	r := activeReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)

	// first call
	_, err := uc.CheckOut(context.Background(), "res-1", "drv-1", "co-dup")
	require.NoError(t, err)

	// second call — idempotent
	result, err := uc.CheckOut(context.Background(), "res-1", "drv-1", "co-dup")
	require.NoError(t, err)
	assert.Equal(t, "res-1", result.ReservationID)
}

func TestCheckOut_WrongDriver(t *testing.T) {
	rdb := setupRedis(t)
	r := activeReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CheckOut(context.Background(), "res-1", "drv-other", "co-key-2")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by driver")
}

func TestCheckOut_WrongStatus(t *testing.T) {
	rdb := setupRedis(t)
	r := confirmedReservation()
	repo := &mockRepo{reservation: r}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	_, err := uc.CheckOut(context.Background(), "res-1", "drv-1", "co-key-3")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot check out")
}

// --- GetReservation / GetActiveReservation Tests ---

func TestGetReservation_Found(t *testing.T) {
	rdb := setupRedis(t)
	r := confirmedReservation()
	repo := &mockRepo{reservation: r}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	result, err := uc.GetReservation(context.Background(), "res-1")

	require.NoError(t, err)
	assert.Equal(t, "res-1", result.ReservationID)
}

func TestGetReservation_NotFound(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{getErr: errors.New("not found")}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	_, err := uc.GetReservation(context.Background(), "res-999")

	require.Error(t, err)
}

func TestGetActiveReservation_Found(t *testing.T) {
	rdb := setupRedis(t)
	r := activeReservation()
	repo := &mockRepo{reservation: r}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	result, err := uc.GetActiveReservation(context.Background(), "drv-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusActive, result.Status)
}

// --- GetAvailability Tests ---

func TestGetAvailability_Car(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{available: 120}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	total, avail, occupied, _, err := uc.GetAvailability(context.Background(), domain.VehicleTypeCar)

	require.NoError(t, err)
	assert.Equal(t, int32(150), total)
	assert.Equal(t, int32(120), avail)
	assert.Equal(t, int32(30), occupied)
}

func TestGetAvailability_Motorcycle(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{available: 200}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	total, avail, _, _, err := uc.GetAvailability(context.Background(), domain.VehicleTypeMotorcycle)

	require.NoError(t, err)
	assert.Equal(t, int32(250), total)
	assert.Equal(t, int32(200), avail)
}

// --- ListAvailableSpots Tests ---

func TestListAvailableSpots(t *testing.T) {
	rdb := setupRedis(t)
	spots := []domain.Spot{
		{SpotID: "s1", Floor: 1, SpotNumber: 1, VehicleType: domain.VehicleTypeCar},
		{SpotID: "s2", Floor: 1, SpotNumber: 2, VehicleType: domain.VehicleTypeCar},
	}
	repo := &mockRepo{spots: spots}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	result, err := uc.ListAvailableSpots(context.Background(), domain.VehicleTypeCar, 1)

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// --- ExpireReservations Tests ---

func TestExpireReservations_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	expired := []domain.Reservation{
		{ReservationID: "res-a", Spot: *testSpot(), Status: domain.StatusConfirmed},
		{ReservationID: "res-b", Spot: *testSpot(), Status: domain.StatusConfirmed},
	}
	repo := &mockRepo{reservations: expired}
	locker := &mockLocker{}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, locker, pub, rdb)
	err := uc.ExpireReservations(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 2, pub.called)
}

func TestExpireReservations_NoExpired(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{reservations: nil}
	pub := &mockPublisher{}

	uc := newTestUsecase(repo, &mockLocker{}, pub, rdb)
	err := uc.ExpireReservations(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, pub.called)
}

func TestExpireReservations_ListError(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{expiredErr: errors.New("db error")}

	uc := newTestUsecase(repo, &mockLocker{}, &mockPublisher{}, rdb)
	err := uc.ExpireReservations(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "list expired")
}
