package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/pkg/config"
	"github.com/edysupardi/parkirpintar/pkg/database"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/lock"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	pkgredis "github.com/edysupardi/parkirpintar/pkg/redis"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/handler"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/repository"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/usecase"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(logger.Config{
		Service: "reservation",
		Level:   "info",
	})

	// database
	db, err := database.New(ctx, database.Config{
		Host:         cfg.Database.Host,
		Port:         cfg.Database.Port,
		Name:         cfg.Database.Name,
		User:         cfg.Database.User,
		Password:     cfg.Database.Password,
		SSLMode:      cfg.Database.SSLMode,
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
	})
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// redis
	redisCfg := pkgredis.Config{
		Host: cfg.Redis.Addr,
		Port: 6379,
	}
	_, err = pkgredis.New(redisCfg)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to redis")
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	defer rdb.Close()

	// dependencies
	repo := repository.New(db.Pool())
	locker := lock.New(rdb)
	idempotencyStore := idempotency.New(rdb)

	// stub publisher — replaced when mq package is implemented
	publisher := &stubPublisher{}

	uc := usecase.New(repo, locker, publisher, idempotencyStore, log)

	// expiry background job
	go runExpiryJob(ctx, uc, log)

	// gRPC server — no auth here, gateway is the auth boundary
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(logger.UnaryServerLogger(log)))

	reservationv1.RegisterReservationServiceServer(srv, handler.New(uc))
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	addr := fmt.Sprintf(":%d", cfg.Services.ReservationGRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to listen")
	}

	log.Info(ctx).Str("addr", addr).Msg("reservation service starting")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatal(ctx).Err(err).Msg("grpc server error")
		}
	}()

	<-quit
	log.Info(ctx).Msg("shutting down")
	srv.GracefulStop()
}

func runExpiryJob(ctx context.Context, uc *usecase.ReservationUsecase, log logger.Logger) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := uc.ExpireReservations(ctx); err != nil {
				log.Error(ctx).Err(err).Msg("expiry job failed")
			}
		}
	}
}

// stubPublisher is used until pkg/mq is implemented.
type stubPublisher struct{}

func (s *stubPublisher) PublishReservationConfirmed(_ context.Context, _ domain.Reservation) error {
	return nil
}
func (s *stubPublisher) PublishReservationExpired(_ context.Context, _ domain.Reservation) error {
	return nil
}
func (s *stubPublisher) PublishReservationCancelled(_ context.Context, _ domain.Reservation) error {
	return nil
}
func (s *stubPublisher) PublishCheckInDetected(_ context.Context, _ domain.Reservation) error {
	return nil
}
func (s *stubPublisher) PublishCheckOutCompleted(_ context.Context, _ domain.Reservation) error {
	return nil
}
