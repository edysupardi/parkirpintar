package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/edysupardi/parkirpintar/pkg/config"
	"github.com/edysupardi/parkirpintar/pkg/database"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/billing/internal/handler"
	"github.com/edysupardi/parkirpintar/services/billing/internal/repository"
	"github.com/edysupardi/parkirpintar/services/billing/internal/usecase"
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

	log := logger.New(logger.Config{Service: "billing", Level: "info"})

	db, err := database.New(ctx, database.Config{
		Host:         cfg.Database.Host,
		Port:         cfg.Database.Port,
		Name:         cfg.Database.Name,
		User:         cfg.Database.User,
		Password:     cfg.Database.Password,
		SSLMode:      cfg.Database.SSLMode,
		MaxOpenConns: int32(cfg.Database.MaxOpenConns),
		MaxIdleConns: int32(cfg.Database.MaxIdleConns),
	})
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
	defer rdb.Close()

	repo := repository.New(db.Pool())
	idempotencyStore := idempotency.New(rdb)
	uc := usecase.New(repo, idempotencyStore, log)

	validator := auth.New(cfg.JWT.Secret)
	srv := grpc.NewServer(grpc.UnaryInterceptor(validator.UnaryInterceptor))

	billingv1.RegisterBillingServiceServer(srv, handler.New(uc))
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	addr := fmt.Sprintf(":%d", 50052)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to listen")
	}

	log.Info(ctx).Str("addr", addr).Msg("billing service starting")

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
