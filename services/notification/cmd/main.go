package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	notificationv1 "github.com/edysupardi/parkirpintar/gen/notification/v1"
	"github.com/edysupardi/parkirpintar/pkg/config"
	"github.com/edysupardi/parkirpintar/pkg/database"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/edysupardi/parkirpintar/pkg/tracer"
	"github.com/edysupardi/parkirpintar/services/notification/internal/handler"
	"github.com/edysupardi/parkirpintar/services/notification/internal/provider"
	"github.com/edysupardi/parkirpintar/services/notification/internal/repository"
	"github.com/edysupardi/parkirpintar/services/notification/internal/subscriber"
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

	log := logger.New(logger.Config{Service: "notification", Level: "info"})

	_, tracerShutdown, err := tracer.Init(ctx, "notification")
	if err != nil {
		log.Warn(ctx).Err(err).Msg("failed to init tracer")
	} else {
		defer func() { _ = tracerShutdown(ctx) }()
	}

	push := provider.NewStubPush(log)
	email := provider.NewStubEmail(log)

	// database for notification history
	var repo *repository.Repository
	db, dbErr := database.New(ctx, database.Config{
		Host:         cfg.Database.Host,
		Port:         cfg.Database.Port,
		Name:         cfg.Database.Name,
		User:         cfg.Database.User,
		Password:     cfg.Database.Password,
		SSLMode:      cfg.Database.SSLMode,
		MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns,
	})
	if dbErr != nil {
		log.Warn(ctx).Err(dbErr).Msg("failed to connect to database, notification history disabled")
	} else {
		defer db.Close()
		repo = repository.New(db.Pool())
	}

	// MQ consumer for all reservation events
	notifHandler := handler.New(push, email, repo, log)
	mqConsumer, err := mq.NewConsumer(cfg.RabbitMQ.URL)
	if err != nil {
		log.Warn(ctx).Err(err).Msg("failed to connect to RabbitMQ, notification subscriber disabled")
	} else {
		defer mqConsumer.Close()
		sub := subscriber.New(notifHandler, log)
		if err := sub.Register(mqConsumer); err != nil {
			log.Warn(ctx).Err(err).Msg("failed to register notification subscriber")
		} else {
			log.Info(ctx).Msg("notification MQ subscriber registered")
		}
	}


	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(logger.UnaryServerLogger(log)))

	notificationv1.RegisterNotificationServiceServer(srv, notifHandler)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	addr := fmt.Sprintf(":%d", cfg.Services.NotificationGRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to listen")
	}

	log.Info(ctx).Str("addr", addr).Msg("notification service starting")

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
