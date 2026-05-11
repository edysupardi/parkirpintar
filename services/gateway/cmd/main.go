package main

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	gatewayv1 "github.com/edysupardi/parkirpintar/gen/gateway/v1"
	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	presencev1 "github.com/edysupardi/parkirpintar/gen/presence/v1"
	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/edysupardi/parkirpintar/pkg/config"
	"github.com/edysupardi/parkirpintar/pkg/database"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/gateway/internal/handler"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
)

//go:embed swagger.json
var swaggerJSON []byte

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(logger.Config{Service: "gateway", Level: "info"})

	// database (for auth endpoints)
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

	dial := func(addr string) *grpc.ClientConn {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
		)
		if err != nil {
			log.Fatal(ctx).Err(err).Str("addr", addr).Msg("failed to connect")
		}
		return conn
	}

	reservationConn := dial(fmt.Sprintf("%s:%d", cfg.Services.ReservationHost, cfg.Services.ReservationGRPCPort))
	defer reservationConn.Close()
	billingConn := dial(fmt.Sprintf("%s:%d", cfg.Services.BillingHost, cfg.Services.BillingGRPCPort))
	defer billingConn.Close()
	paymentConn := dial(fmt.Sprintf("%s:%d", cfg.Services.PaymentHost, cfg.Services.PaymentGRPCPort))
	defer paymentConn.Close()
	presenceConn := dial(fmt.Sprintf("%s:%d", cfg.Services.PresenceHost, cfg.Services.PresenceGRPCPort))
	defer presenceConn.Close()

	validator := auth.New(cfg.JWT.Secret)

	extraH := handler.NewExtra(db.Pool(), redis.NewClient(cfg.Redis.RedisOptions()), validator, cfg.Midtrans.ServerKey)

	h := handler.New(
		reservationv1.NewReservationServiceClient(reservationConn),
		billingv1.NewBillingServiceClient(billingConn),
		paymentv1.NewPaymentServiceClient(paymentConn),
		presencev1.NewPresenceServiceClient(presenceConn),
		extraH,
	)

	// gRPC server for direct gRPC clients (with auth interceptor)
	grpcSrv := grpc.NewServer(grpc.UnaryInterceptor(validator.UnaryInterceptor))
	gatewayv1.RegisterGatewayServiceServer(grpcSrv, h)
	grpc_health_v1.RegisterHealthServer(grpcSrv, health.NewServer())
	reflection.Register(grpcSrv)

	grpcAddr := "0.0.0.0:50050" // #nosec G102 — intentional, internal gRPC server
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to listen gRPC")
	}

	// ── grpc-gateway HTTP mux ─────────────────────────────────────────────────
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				UseProtoNames:   true,
				EmitUnpopulated: false,
			},
		}),
	)

	if err := gatewayv1.RegisterGatewayServiceHandlerServer(ctx, mux, h); err != nil {
		log.Fatal(ctx).Err(err).Msg("failed to register gateway handler")
	}

	httpMux := http.NewServeMux()
	httpMux.Handle("/v1/", mux)

	// auth endpoints (public, no JWT required)
	authH := handler.NewAuth(db.Pool(), validator, cfg.JWT.ExpiryHours)
	httpMux.HandleFunc("/v1/auth/register", authH.Register)
	httpMux.HandleFunc("/v1/auth/login", authH.Login)

	// extra endpoints
	httpMux.HandleFunc("/v1/reservations/history", extraH.ReservationHistory)
	httpMux.HandleFunc("/v1/reservations/confirm-payment", extraH.ConfirmBookingPayment)
	httpMux.HandleFunc("/v1/reservations/pending-payment", extraH.GetPendingPayment)
	httpMux.HandleFunc("/v1/parking/spots", extraH.ListAvailableSpots)
	httpMux.HandleFunc("/v1/payments/simulate-settle", extraH.SimulateSettle)

	httpMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	httpMux.HandleFunc("/swagger.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(swaggerJSON)
	})
	httpMux.HandleFunc("/swagger/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><a href="https://petstore.swagger.io/?url=http://localhost:8080/swagger.json">Open Swagger UI</a></body></html>`))
	})

	httpAddr := ":8080"
	limiter := handler.NewRateLimiter(100, 1*time.Minute)
	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           logger.HTTPRequestLogger(log)(limiter.Middleware(corsMiddleware(validator.HTTPMiddleware(httpMux)))),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Info(ctx).Str("grpc", grpcAddr).Str("http", httpAddr).Msg("gateway starting")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatal(ctx).Err(err).Msg("gRPC server error")
		}
	}()
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(ctx).Err(err).Msg("HTTP server error")
		}
	}()

	<-quit
	log.Info(ctx).Msg("shutting down")
	grpcSrv.GracefulStop()
	_ = httpSrv.Shutdown(ctx)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("CORS_ORIGIN")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
