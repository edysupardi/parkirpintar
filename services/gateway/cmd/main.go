package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	gatewayv1 "github.com/edysupardi/parkirpintar/gen/gateway/v1"
	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	presencev1 "github.com/edysupardi/parkirpintar/gen/presence/v1"
	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/edysupardi/parkirpintar/pkg/config"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/gateway/internal/handler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	log := logger.New(logger.Config{Service: "gateway", Level: "info"})

	// dial downstream services
	reservationConn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", cfg.Services.ReservationGRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to reservation service")
	}
	defer reservationConn.Close()

	billingConn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", cfg.Services.BillingGRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to billing service")
	}
	defer billingConn.Close()

	paymentConn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", cfg.Services.PaymentGRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to payment service")
	}
	defer paymentConn.Close()

	presenceConn, err := grpc.NewClient(fmt.Sprintf("localhost:%d", cfg.Services.PresenceGRPCPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to connect to presence service")
	}
	defer presenceConn.Close()

	h := handler.New(
		reservationv1.NewReservationServiceClient(reservationConn),
		billingv1.NewBillingServiceClient(billingConn),
		paymentv1.NewPaymentServiceClient(paymentConn),
		presencev1.NewPresenceServiceClient(presenceConn),
	)

	validator := auth.New(cfg.JWT.Secret)
	srv := grpc.NewServer(grpc.UnaryInterceptor(validator.UnaryInterceptor))

	gatewayv1.RegisterGatewayServiceServer(srv, h)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	reflection.Register(srv)

	addr := fmt.Sprintf(":%d", 50050)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to listen")
	}

	log.Info(ctx).Str("addr", addr).Msg("gateway service starting")

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
