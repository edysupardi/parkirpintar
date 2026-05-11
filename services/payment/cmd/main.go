package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	"github.com/edysupardi/parkirpintar/pkg/config"
	"github.com/edysupardi/parkirpintar/pkg/database"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/edysupardi/parkirpintar/services/payment/internal/gateway"
	"github.com/edysupardi/parkirpintar/services/payment/internal/handler"
	"github.com/edysupardi/parkirpintar/services/payment/internal/repository"
	"github.com/edysupardi/parkirpintar/services/payment/internal/usecase"
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

	log := logger.New(logger.Config{Service: "payment", Level: "info"})

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

	rdb := redis.NewClient(cfg.Redis.RedisOptions())
	defer rdb.Close()

	repo := repository.New(db.Pool())
	idempotencyStore := idempotency.New(rdb)

	var gw domain.PaymentGateway
	if cfg.Feature.PaymentProvider == "mock" {
		gw = gateway.NewMock(cfg.Midtrans.ServerKey)
		log.Info(ctx).Msg("using mock payment gateway")
	} else {
		gw = gateway.NewMidtrans(cfg.Midtrans.ServerKey, cfg.Midtrans.Env == "production")
		log.Info(ctx).Msg("using midtrans payment gateway")
	}
	uc := usecase.New(repo, gw, idempotencyStore, cfg.Midtrans.ServerKey, log)

	
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(logger.UnaryServerLogger(log)))

	paymentv1.RegisterPaymentServiceServer(srv, handler.New(uc))
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	addr := fmt.Sprintf(":%d", cfg.Services.PaymentGRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to listen")
	}

	log.Info(ctx).Str("addr", addr).Msg("payment service starting")

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
