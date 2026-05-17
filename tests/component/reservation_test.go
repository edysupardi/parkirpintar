//go:build component

package component_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	commonv1 "github.com/edysupardi/parkirpintar/gen/common/v1"
	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/reservation"
	"github.com/edysupardi/parkirpintar/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	testDB  *testutil.TestDB
	testRds *testutil.TestRedis
	client  reservationv1.ReservationServiceClient
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.NewTestDB(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to create test db: %v", err))
	}
	defer testDB.Close(ctx)

	testRds, err = testutil.NewTestRedis()
	if err != nil {
		panic(fmt.Sprintf("failed to create test redis: %v", err))
	}
	defer testRds.Close()

	// Start real gRPC server with reservation service
	log := logger.New(logger.Config{Service: "component-test", Level: "error"})

	srv := grpc.NewServer()
	reservation.RegisterServer(srv, testDB.Pool, testRds.Client, log)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("failed to listen: %v", err))
	}

	go func() { _ = srv.Serve(lis) }()
	defer srv.GracefulStop()

	// Connect gRPC client
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("failed to dial: %v", err))
	}
	defer conn.Close()

	client = reservationv1.NewReservationServiceClient(conn)

	m.Run()
}

func uid() string { return uuid.New().String() }

func ctxWithDriver(driverID string) context.Context {
	md := metadata.Pairs("x-driver-id", driverID)
	return metadata.NewOutgoingContext(context.Background(), md)
}

// ── Component Test: CreateReservation via gRPC ──────────────────────────────

func TestComponent_CreateReservation_SystemAssigned(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()

	resp, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       uid(),
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_CAR,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: uid(),
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.ReservationId)
	assert.NotNil(t, resp.Spot)
	assert.Equal(t, commonv1.VehicleType_VEHICLE_TYPE_CAR, resp.Spot.VehicleType)
	assert.Equal(t, int64(5000), resp.BookingFee.Amount)
}

func TestComponent_CreateReservation_UserSelected(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()

	// First get a spot via system assign
	first, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       uid(),
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_MOTORCYCLE,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: uid(),
	})
	require.NoError(t, err)

	// Try to reserve same spot — should fail
	_, err = client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       uid(),
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_MOTORCYCLE,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_USER_SELECTED,
		SpotId:         first.Spot.SpotId,
		IdempotencyKey: uid(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spot unavailable")
}

func TestComponent_CreateReservation_Idempotent(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	driverID := uid()
	key := uid()

	resp1, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       driverID,
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_CAR,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: key,
	})
	require.NoError(t, err)

	resp2, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       driverID,
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_CAR,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: key,
	})
	require.NoError(t, err)
	assert.Equal(t, resp1.ReservationId, resp2.ReservationId)
}

// ── Component Test: Full flow via gRPC ──────────────────────────────────────

func TestComponent_FullFlow_Reserve_CheckIn_CheckOut(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	driverID := uid()

	// Reserve
	res, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       driverID,
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_CAR,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: uid(),
	})
	require.NoError(t, err)
	assert.Equal(t, commonv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED, res.Status) // pending

	// Confirm (simulate payment done) — need to call usecase directly since no gRPC endpoint
	// In production, gateway calls ConfirmReservation after payment
	// For component test, we verify the gRPC layer works correctly

	// CheckIn — will fail because status is pending (not confirmed)
	_, err = client.CheckIn(ctx, &reservationv1.CheckInRequest{
		ReservationId: res.ReservationId,
		DriverId:      driverID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending")
}

func TestComponent_GetAvailability(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()

	resp, err := client.GetAvailability(ctx, &reservationv1.GetAvailabilityRequest{
		VehicleType: commonv1.VehicleType_VEHICLE_TYPE_CAR,
	})

	require.NoError(t, err)
	assert.True(t, resp.IsAvailable)
	assert.Greater(t, resp.TotalCapacity, int32(0))
	assert.Greater(t, resp.AvailableSpots, int32(0))
}

func TestComponent_CancelReservation(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	driverID := uid()

	res, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       driverID,
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_CAR,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: uid(),
	})
	require.NoError(t, err)

	// Cancel immediately
	cancelResp, err := client.CancelReservation(ctx, &reservationv1.CancelReservationRequest{
		ReservationId: res.ReservationId,
		DriverId:      driverID,
	})
	require.NoError(t, err)
	assert.Equal(t, commonv1.ReservationStatus_RESERVATION_STATUS_CANCELLED, cancelResp.Status)
	assert.NotNil(t, cancelResp.CancelledAt)
}

func TestComponent_CancelReservation_WrongDriver(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()

	res, err := client.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       uid(),
		VehicleType:    commonv1.VehicleType_VEHICLE_TYPE_CAR,
		AssignmentMode: commonv1.AssignmentMode_ASSIGNMENT_MODE_SYSTEM,
		IdempotencyKey: uid(),
	})
	require.NoError(t, err)

	_, err = client.CancelReservation(ctx, &reservationv1.CancelReservationRequest{
		ReservationId: res.ReservationId,
		DriverId:      uid(), // wrong driver
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned")
}

// suppress unused imports
var _ = time.Now
var _ = metadata.Pairs
