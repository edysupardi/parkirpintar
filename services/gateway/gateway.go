package gateway

import (
	"context"
	"net"
	"net/http"
	"time"

	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	gatewayv1 "github.com/edysupardi/parkirpintar/gen/gateway/v1"
	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	presencev1 "github.com/edysupardi/parkirpintar/gen/presence/v1"
	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/edysupardi/parkirpintar/services/gateway/internal/handler"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

type TestServerConfig struct {
	Pool           *pgxpool.Pool
	Redis          *redis.Client
	Validator      *auth.Validator
	JWTExpiryHours int
	ServerKey      string
	ReservationAddr string
	BillingAddr     string
	PaymentAddr     string
	PresenceAddr    string
}

func StartTestHTTPServer(ctx context.Context, cfg TestServerConfig) (addr string, cleanup func()) {
	dial := func(target string) *grpc.ClientConn {
		if target == "" {
			return nil
		}
		conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			panic(err)
		}
		return conn
	}

	resConn := dial(cfg.ReservationAddr)
	billConn := dial(cfg.BillingAddr)
	payConn := dial(cfg.PaymentAddr)
	presConn := dial(cfg.PresenceAddr)

	extraH := handler.NewExtra(cfg.Pool, cfg.Redis, cfg.Validator, cfg.ServerKey)

	var presClient presencev1.PresenceServiceClient
	if presConn != nil {
		presClient = presencev1.NewPresenceServiceClient(presConn)
	}

	h := handler.New(
		reservationv1.NewReservationServiceClient(resConn),
		billingv1.NewBillingServiceClient(billConn),
		paymentv1.NewPaymentServiceClient(payConn),
		presClient,
		extraH,
	)

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: false},
		}),
	)
	if err := gatewayv1.RegisterGatewayServiceHandlerServer(ctx, mux, h); err != nil {
		panic(err)
	}

	httpMux := http.NewServeMux()
	httpMux.Handle("/v1/", mux)

	authH := handler.NewAuth(cfg.Pool, cfg.Validator, cfg.JWTExpiryHours)
	httpMux.HandleFunc("/v1/auth/register", authH.Register)
	httpMux.HandleFunc("/v1/auth/login", authH.Login)
	httpMux.HandleFunc("/v1/reservations/confirm-payment", extraH.ConfirmBookingPayment)
	httpMux.HandleFunc("/v1/parking/spots", extraH.ListAvailableSpots)
	httpMux.HandleFunc("/v1/payments/simulate-settle", extraH.SimulateSettle)

	httpMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	srv := &http.Server{
		Handler:           cfg.Validator.HTTPMiddleware(httpMux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(lis) }()

	cleanup = func() {
		_ = srv.Close()
		if resConn != nil {
			resConn.Close()
		}
		if billConn != nil {
			billConn.Close()
		}
		if payConn != nil {
			payConn.Close()
		}
		if presConn != nil {
			presConn.Close()
		}
	}

	return "http://" + lis.Addr().String(), cleanup
}
