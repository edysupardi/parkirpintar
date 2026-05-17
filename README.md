# ParkirPintar - Backend Solution

> Smart Parking Marketplace - Assessment 2026  
> Author: Edy Supardi

---

## 📌 Table of Contents

1. [Project Overview](#1-project-overview)
2. [Assumptions & Constraints](#2-assumptions--constraints)
3. [Capacity Planning](#3-capacity-planning)
4. [High Level Architecture](#4-high-level-architecture-hld)
5. [Low Level Architecture](#5-low-level-architecture-lld)
6. [Architecture Pattern](#6-architecture-pattern)
7. [ERD](#7-erd--entity-relationship-diagram)
8. [Technology Decisions](#8-technology-decisions--justifications)
9. [AWS Infrastructure](#9-aws-infrastructure)
10. [Deployment Strategy](#10-deployment-strategy)
11. [CI/CD Pipeline](#11-cicd-pipeline)
12. [Observability](#12-observability)
13. [Business Rules Reference](#13-business-rules-reference)
14. [Testing Strategy](#14-testing-strategy)
15. [Third-Party Libraries](#15-third-party-libraries--tools)
16. [How to Run](#16-how-to-run)

---

## 1. Project Overview

ParkirPintar adalah sistem smart parking berbasis microservices yang memungkinkan driver mereservasi spot parkir di satu area parkir terpusat. Dibangun sebagai mini app di dalam super app atau sebagai standalone service.

**Karakteristik utama:**
- Single parking area, centralized inventory - 5 lantai, 30 mobil + 50 motor per lantai (total: 150 mobil, 250 motor)
- Reservation dengan Redis inventory lock untuk mencegah double-booking
- Billing dihitung dari actual parking session duration
- Real-time location tracking via presence service
- Payment via Midtrans: QRIS (scannable by all e-wallets), Virtual Account
- Notification via FCM (push) dan Amazon SES (email) — stub-able untuk testing

---

## 2. Assumptions & Constraints

| # | Asumsi | Alasan |
|---|--------|--------|
| A1 | Sistem hanya melayani 1 area parkir (single-tenant) | Sesuai soal: "single, fixed parking area" |
| A2 | Tidak ada Host onboarding - spot sudah pre-seeded di database | Sesuai soal: "centralized inventory", tidak ada multi-host |
| A3 | Driver register/login via gateway service (JWT issued by system) | Self-contained auth untuk assessment |
| A4 | Geofence / auto check-in tidak diimplementasi — dihapus dari requirement | Tidak ada di requirement asesmen final |
| A5 | Timezone sistem adalah WIB (UTC+7) untuk overnight fee | Konteks Jakarta |
| A6 | "Crossing midnight" = session melewati 00:00 WIB | Kalkulasi overnight fee |
| A7 | Satu driver hanya boleh punya 1 active reservation pada satu waktu | Mencegah abuse |
| A8 | System-assigned: pilih lantai terbawah, nomor terkecil yang tersedia | Deterministic dan fair |
| A9 | Midtrans sandbox digunakan untuk demo | Assessment environment |
| A10 | Notification stub menggantikan FCM/SES di environment testing | Kemudahan E2E test |

### Out of Scope
- Multi-area search dan discovery
- Host management dan onboarding
- Dynamic/surge pricing
- Subscription atau membership plans
- Refund processing

---

## 3. Capacity Planning

Dilakukan untuk menjustifikasi keputusan arsitektur - khususnya mengapa tidak butuh infrastruktur yang over-engineered.

```
Total kapasitas    : 400 spot (150 mobil + 250 motor)
Asumsi turnover    : 4x per hari per spot
Total transaksi    : ~1.600 reservasi/hari

Peak hour (08:00–10:00 dan 17:00–19:00):
  ~30% traffic dalam 4 jam = 480 reservasi
  = 120 reservasi/jam peak
  = ~0.033 RPS

Location updates (presence):
  Max active sessions : 400
  Interval            : 30 detik
  = 400 / 30 ≈ 13 writes/detik
```

> Sistem ini adalah **LOW traffic system**. Ini menjustifikasi pilihan ECS Fargate (bukan EKS), Aurora Serverless v2 (bukan RDS provisioned), dan RabbitMQ on ECS (bukan Kafka/Amazon MQ managed).

---

## 4. High Level Design (HLD)

```mermaid
graph TD
    App([Mobile App]) -->|HTTPS| R53[Route 53]
    R53 --> ACM[ACM SSL]
    ACM --> ALB[ALB External]
    ALB -->|REST/JSON| GW[API Gateway Service]
    GW -->|gRPC/NLB| RS[Reservation Service]
    GW -->|gRPC/NLB| BS[Billing Service]
    GW -->|gRPC/NLB| PS[Presence Service]
    RS & BS & PS --> MQ[RabbitMQ on ECS]
    MQ --> PAY[Payment Service]
    MQ --> NOTIF[Notification Service]
    MQ --> BC[Billing Consumer]
    PAY --> MID([Midtrans])
    NOTIF --> FCM([FCM Google])
    NOTIF --> SES([Amazon SES])
    RS & BS & PAY --> DB[(Aurora Serverless v2)]
    RS & BS --> REDIS[(ElastiCache Redis)]
```

### Komponen Utama

| Komponen | Teknologi | Fungsi |
|----------|-----------|--------|
| API Gateway Service | Go + gRPC-gateway | Single entry point, auth, routing |
| Reservation Service | Go + gRPC | Core business: book, cancel, expire |
| Billing Service | Go + gRPC | Pricing engine, invoice generation |
| Payment Service | Go + gRPC | Midtrans integration, webhook handler |
| Presence Service | Go + gRPC streaming | Location tracking, bidirectional streaming |
| Notification Service | Go + gRPC | FCM push + SES email (stub-able) |
| Aurora Serverless v2 | PostgreSQL-compatible | Primary datastore |
| ElastiCache Serverless | Redis 7.x | Distributed lock, idempotency, cache |
| RabbitMQ on ECS | RabbitMQ 3.13 | Async event bus antar services (Cloud Map DNS) |

---

## 5. Low Level Design (LLD)

### Service Communication

```mermaid
flowchart TD
    App([Mobile App])
    GW[API Gateway Service]
    RS[Reservation Service]
    BS[Billing Service]
    PAY[Payment Service]
    PS[Presence Service]
    MID([Midtrans API - external])

    App -->|HTTPS REST JSON| GW
    GW -->|gRPC HTTP2 via NLB internal| RS
    GW -->|gRPC HTTP2 via NLB internal| PS
    RS -->|gRPC| BS
    BS -->|gRPC| PAY
    PAY -->|HTTPS REST| MID
    PS <-->|bidirectional streaming gRPC| GW
```

---

### Event Flow — Async via RabbitMQ

```mermaid
flowchart LR
    subgraph PUB[Publishers]
        RS[Reservation Service]
        PS[Presence Service]
        PAY[Payment Service]
    end

    subgraph MQ[RabbitMQ on ECS]
        E1([ReservationConfirmed])
        E2([ReservationExpired])
        E3([CheckInDetected])
        E4([CheckOutCompleted])
        E5([PaymentSettled])
    end

    subgraph CON[Consumers]
        NOTIF[Notification Service]
        BS[Billing Service]
        RS2[Reservation Service]
    end

    RS -->|publishes| E1
    RS -->|publishes| E2
    RS -->|publishes| E4
    PS -->|publishes| E3
    PAY -->|publishes| E5

    E1 -->|FCM + Email konfirmasi| NOTIF
    E2 -->|FCM + Email expired| NOTIF
    E2 -->|release spot, booking fee forfeited| BS
    E3 -->|update status ke ACTIVE| RS2
    E3 -->|FCM welcome| NOTIF
    E4 -->|generate invoice| BS
    E4 -->|FCM + Email receipt| NOTIF
    E5 -->|close invoice| BS
    E5 -->|FCM + Email payment success| NOTIF
```

---

### Reservation State Machine

```mermaid
stateDiagram-v2
    [*] --> PENDING : Driver reserves

    PENDING --> CONFIRMED : Booking fee 5.000 charged\nspot locked

    PENDING --> FAILED : Payment failed\nspot not locked

    CONFIRMED --> ACTIVE : Driver check-in\nmanual

    CONFIRMED --> EXPIRED : No check-in within 1 hour\nbooking fee forfeited

    CONFIRMED --> CANCELLED : Driver cancels\nbooking fee forfeited

    ACTIVE --> COMPLETED : Driver checkout\ninvoice generated (parking + overnight only)

    EXPIRED --> [*]
    CANCELLED --> [*]
    COMPLETED --> [*]
    FAILED --> [*]
```

---

### Redis Inventory Lock — Anti Double-Booking

```mermaid
flowchart TD
    A([Driver request reserve spot X])
    B["SET spot:{spot_id}:lock {reservation_id}\nNX EX 3600"]
    C[Insert reservation ke PostgreSQL\ndalam DB transaction]
    D([Reservation CONFIRMED\nlock held for 1 hour])
    E["DEL spot:{spot_id}:lock\nrollback lock"]
    Z([Return error - spot unavailable])
    F([Reservation expires after 1 hour])
    G([Lock auto-released by Redis TTL\nspot available again])

    A --> B
    B -->|key belum ada - SUCCESS| C
    B -->|key sudah ada - FAIL| Z
    C -->|DB success| D
    C -->|DB failed| E
    E --> Z
    F -->|EX 3600 TTL expires| G
```

---

### Idempotency Pattern

```mermaid
flowchart TD
    A([Incoming Request\nHeader - Idempotency-Key: uuid-v4])
    B["GET idempotency:{key} dari Redis"]
    C([Return cached response\ntidak proses ulang])
    D[Proses request normal]
    E["SET idempotency:{key} response\nEX 86400 - 24 jam"]
    F([Return response ke client])

    A --> B
    B -->|HIT - key ada| C
    B -->|MISS - key tidak ada| D
    D --> E
    E --> F
```

> Berlaku untuk: `CreateReservation`, `CheckOut/GenerateInvoice`, `CreateTransaction`, `HandleWebhook`

---

### Circuit Breaker — Graceful Degradation

```mermaid
stateDiagram-v2
    [*] --> CLOSED : Normal operation\nall requests pass through

    CLOSED --> OPEN : Failure threshold exceeded\ne.g error rate above 50pct

    OPEN --> HALF_OPEN : After timeout period\nallow one probe request

    HALF_OPEN --> CLOSED : Probe request success\nresume normal operation

    HALF_OPEN --> OPEN : Probe request failed\nback to open state
```

**Non-core service failures** — Notification, Presence:
- Log error, lanjutkan main flow
- Circuit breaker: CLOSED → OPEN → HALF-OPEN

**Core service failures** — Reservation, Billing:
- Return error ke client dengan retry guidance
- Dead letter queue untuk failed events

**Implementasi:** `pkg/circuitbreaker` (sony/gobreaker) di-wire sebagai gRPC client interceptor di gateway — setiap call ke downstream service (reservation, billing, payment, presence) di-wrap circuit breaker dengan threshold 50% failure rate.
---

### Pricing Engine Rules

| Rule | Value |
|------|-------|
| Booking fee | 5.000 IDR — charged upfront saat reservation (non-refundable) |
| Hourly rate | 5.000 IDR/jam — first hour + each **started** hour |
| Overnight fee | 20.000 IDR flat — jika session crossing midnight WIB |
| Overstay | Tidak ada penalty — billing normal (hourly rate) |

**Booking fee = commitment fee:**
- Charged saat reserve, sebelum spot di-lock
- Non-refundable dalam kondisi apapun (cancel, no-show, expire)
- Menjamin revenue owner meskipun driver tidak datang
- Tidak masuk invoice checkout — invoice hanya parking + overnight

**Cancellation policy:**

| Kondisi | Additional Fee |
|---------|---------------|
| Cancel kapanpun | 0 IDR (booking fee 5.000 sudah hangus) |
| No-show > 1 jam | 0 IDR + auto-expire (booking fee sudah hangus) |

**Contoh kalkulasi:**

```
Park 1j 1m:
  booking fee  = 5.000 IDR (charged at reserve)
  parking fee  = ceil(61/60) = 2 jam × 5.000 = 10.000 IDR
  total invoice at checkout = 10.000 IDR

Park 23:00 – 01:00:
  booking fee  = 5.000 IDR (charged at reserve)
  parking fee  = 2 jam × 5.000 = 10.000 IDR
  overnight    = 20.000 IDR
  total invoice at checkout = 30.000 IDR

Total revenue owner (park 23:00-01:00):
  booking fee + invoice = 5.000 + 30.000 = 35.000 IDR
```

---

## 6. Architecture Pattern

Setiap microservice menggunakan **Layered Architecture** dengan dependency flow yang ketat:

```
handler/ → usecase/ → repository/
```

| Layer | Lokasi | Tanggung Jawab |
|-------|--------|----------------|
| `handler/` | `internal/handler/` | Terima gRPC request, validasi input, translate proto ↔ domain |
| `usecase/` | `internal/usecase/` | Business logic dan orchestration — tidak tau detail DB atau gRPC |
| `repository/` | `internal/repository/` | Data access layer — query PostgreSQL dan Redis |
| `domain/` | `internal/domain/` | Struct definitions dan interfaces — zero external dependency |
| `subscriber/` | `internal/subscriber/` | RabbitMQ event consumer — panggil usecase saat terima event |

**Aturan dependency:**
- `handler` boleh panggil `usecase`, tidak boleh langsung query DB
- `usecase` boleh panggil `repository`, tidak boleh tau HTTP/gRPC detail
- `repository` hanya urusan data — tidak ada business logic
- `domain` tidak boleh import package manapun dari project ini

**Struktur per service:**
```
services/{name}/
├── cmd/main.go              ← entry point, wire semua dependency
├── internal/
│   ├── domain/              ← struct + interfaces (zero deps)
│   ├── handler/             ← gRPC handler
│   ├── usecase/             ← business logic
│   ├── repository/          ← data access
│   └── subscriber/          ← MQ event consumer
└── Dockerfile
```

---

## 7. ERD - Entity Relationship Diagram

> Detail ERD: [DB-Diagram](https://dbdiagram.io/d/ParkirPintar-69ecb794c6a36f9c1b7b1899)

```
spots
  ↓
drivers → reservations → location_updates
  ↓           ↓
notif_logs  parking_sessions
                ↓
             invoices
                ↓
             payments
```

---

## 8. Technology Decisions & Justifications

| Komponen | Pilihan | Alternatif | Alasan |
|----------|---------|------------|--------|
| Container | ECS Fargate | EKS, GKE | EKS charge $73/bln hanya control plane. Capacity planning menunjukkan ~0.033 RPS - ECS Fargate jauh lebih cost-efficient untuk skala ini |
| Database | Aurora Serverless v2 | RDS PostgreSQL, DynamoDB | Traffic bersifat peak-hour. Aurora scale 0.5–128 ACU otomatis. DynamoDB tidak cocok untuk skema relasional kompleks ini |
| Cache/Lock | ElastiCache Serverless Redis | Memcached, self-hosted Redis | Redis SET NX adalah primitive paling tepat untuk distributed lock. Soal secara eksplisit menyebut Redis-based lock |
| Message Queue | RabbitMQ on ECS Fargate | Kafka (MSK), SQS+SNS, Amazon MQ | Kafka untuk jutaan events/detik. Sistem ini ~1.600 events/hari - Kafka over-engineering. Amazon MQ managed minimum mq.m5.large (~$216/bln) - overkill untuk dev. RabbitMQ on ECS via Cloud Map service discovery: cost ~$3/bln, same AMQP protocol |
| Load Balancer | ALB (external) + NLB (internal) + client-side LB | ALB only, App Mesh | NLB TCP pass-through untuk gRPC internal dikombinasikan dengan client-side load balancing (DNS round-robin via ECS Service Discovery). ALB untuk REST external karena support WAF dan routing rules. ALB internal bisa distribute gRPC lebih merata tapi tambah latency dan cost — trade-off yang tidak worth untuk skala ini |
| Payment | Midtrans | Xendit, Stripe | Stripe tidak support QRIS/VA Indonesia/e-wallet lokal. Midtrans cover semua dalam satu integrasi|
| IaC | Terraform | AWS CDK, CloudFormation | Industry standard, multi-cloud, AWS provider paling mature, paling mudah di-review oleh siapapun |
| Notification | FCM + Amazon SES | AWS SNS, SendGrid | FCM standard untuk mobile push. SES murah ($0.10/1000 email), native AWS, bounce handling otomatis |

---

## 9. AWS Infrastructure

### Services

| AWS Service | Fungsi | Tier / Sizing |
|-------------|--------|---------------|
| ECS Fargate | Container runtime (6 services + RabbitMQ) | Per-task billing |
| Aurora Serverless v2 | Primary PostgreSQL | 0.5–4 ACU auto-scale |
| ElastiCache Serverless | Redis lock + cache | On-demand |
| RabbitMQ on ECS | Message broker (Cloud Map DNS) | 0.25 vCPU / 512MB |
| ALB | External HTTPS load balancer | - |
| NLB | Internal gRPC load balancer | - |
| ECR | Docker image registry | - |
| Route 53 | DNS | - |
| ACM | SSL certificate | Free |
| CloudWatch | Logs + metrics + alarms | - |
| AWS X-Ray | Distributed tracing | - |
| Secrets Manager | DB creds, API keys | - |

### Estimasi Biaya (Dev Environment)

| Komponen | Est./bulan |
|----------|------------|
| ECS Fargate (7 tasks: 6 services + RabbitMQ) | ~$30–50 |
| Aurora Serverless v2 (0.5 ACU idle) | ~$20–30 |
| ElastiCache Serverless | ~$10–15 |
| ALB + NLB | ~$20 |
| NAT Gateway | ~$30 |
| ECR, CloudWatch, misc | ~$10 |
| **Total Dev** | **~$120–155/bln** |

> Tip: Gunakan `./scripts/destroy-infra.sh` untuk tear down saat tidak dipakai. Cost hanya terhitung saat infra aktif.

### Terraform Structure

```
terraform/
├── modules/
│   ├── networking/      ← VPC, subnets, NAT, security groups
│   ├── ecs/             ← Cluster, task definitions, services
│   ├── rds/             ← Aurora Serverless v2
│   ├── elasticache/     ← Redis Serverless
│   ├── load-balancer/   ← ALB (HTTP/HTTPS) + NLB (gRPC)
│   ├── mq/              ← RabbitMQ on ECS + Cloud Map
│   └── monitoring/      ← CloudWatch dashboard + alarms + SNS
└── environments/
    ├── dev/             ← minimal sizing, HTTP-only
    └── prod/            ← production sizing + HTTPS + auto-scaling
```

---

## 10. Deployment Strategy

| Environment | Strategy | Alasan | Trade-off |
|-------------|----------|--------|-----------|
| Development | Recreate | Clean state setiap deploy menyederhanakan debugging | Downtime ~10 detik saat deploy |
| Staging | Rolling Update | Mirror production behavior, zero downtime | Dua versi bisa berjalan bersamaan sebentar — acceptable di staging |
| Production (app) | Canary | Validasi gradual sebelum full rollout, rollback cepat jika ada issue | Butuh monitoring yang proper, sedikit lebih lambat rollout |
| Production (infra) | Blue/Green | Rollback instant untuk perubahan besar seperti DB migration atau config change | 2x infrastructure cost sementara selama cutover |

### Canary Flow

```
Deploy v2 ke 10% traffic
  → Monitor 30 menit
  → Error rate < 1%? Lanjut ke 50%
  → Error rate > 5%? Auto-rollback ke v1

Deploy v2 ke 50% → Monitor → 100%
```

---

## 11. CI/CD Pipeline

### GitHub Actions Workflows

| Workflow | Trigger | Steps |
|----------|---------|-------|
| `ci.yml` | Push/PR to develop, main | Lint → Unit tests → Integration tests → E2E tests → Build |
| `deploy-dev.yml` | Push to develop (path-based) | OIDC auth → Build → Push ECR → Rolling update ECS |
| `deploy-prod.yml` | Push to main (path-based) / manual | OIDC auth → Build → Push ECR → Canary 10% → Monitor 5min → Full rollout |

### Pipeline Flow

```
Push to develop
  ├── CI: lint + test-pkg + test-integration + test-e2e + build
  └── Deploy Dev: build image → push ECR → rolling update ECS

PR develop → main
  └── CI runs on PR

Merge to main (services/pkg/gen changes only)
  └── Deploy Prod: build → push → canary 10% → monitor → full rollout
      └── Circuit breaker auto-rollback if error rate > threshold
```

### Security

- **No static credentials** — GitHub Actions uses OIDC (OpenID Connect) to assume IAM role
- **Secrets in AWS Secrets Manager** — DB password, JWT secret, Midtrans keys
- **Path-based triggers** — only changed services get rebuilt and deployed

---

## 12. Observability

### The 4 Golden Signals

| Signal | Metric | Alert |
|--------|--------|-------|
| Latency | P99 reservation API | > 500ms |
| Traffic | RPS per service | Spike > 3x baseline |
| Errors | Error rate | > 1% warning, > 5% critical |
| Saturation | CPU ECS task, Redis memory | CPU > 80%, Redis > 70% |

### Structured Logging

Semua service menggunakan **zerolog** (JSON structured logging) dengan fitur:

| Feature | Detail |
|---------|--------|
| Request ID | Auto-generated UUID per request, propagated via `X-Request-ID` header (HTTP) dan gRPC metadata |
| HTTP Request Logger | Method, path, status, latency, IP, user_agent, user_id |
| gRPC Request Logger | Method, status code, latency, request_id |
| Sensitive Masking | Fields `password`, `token`, `secret` di-mask sebagai `[REDACTED]` |
| Log Levels | `debug` (dev), `info` (prod), configurable via env |
| Correlation | Request ID sama di gateway dan downstream service untuk tracing |

**Log format (production):**
```json
{"level":"info","service":"gateway","request_id":"a1b2c3d4","method":"POST","path":"/v1/reservations","status":200,"latency":45.2,"ip":"192.168.1.25","user_agent":"Mozilla/5.0","time":"2026-05-09T14:30:00Z","message":"http request"}
```

**Middleware chain (gateway):**
```
Request Logger → Rate Limiter → CORS → Auth → Handler
```

**gRPC interceptor (internal services):**
```
Logger Interceptor → Handler
```

**gRPC client interceptor (gateway → downstream):**
```
Circuit Breaker → OTel Trace → gRPC Call
```

### Tools

| Tool | Fungsi |
|------|--------|
| CloudWatch Logs | Centralized logs dari semua ECS tasks |
| CloudWatch Metrics | Infrastructure metrics |
| CloudWatch Alarms | Alert + auto-scaling trigger |
| OpenTelemetry SDK | Distributed tracing — stdout exporter (dev), propagasi via TraceContext header |
| otelgrpc | Auto-instrument semua gRPC client calls di gateway |

---

## 13. Business Rules Reference

### Pricing

| Rule | Value |
|------|-------|
| Booking fee | 5.000 IDR (charged upfront saat reserve, non-refundable) |
| Hourly rate | 5.000 IDR/jam (first + each started hour) |
| Overnight fee | 20.000 IDR flat (crossing midnight WIB) |
| Overstay penalty | Tidak ada - billing normal |
| Invoice at checkout | Parking + overnight only (booking fee sudah dibayar) |

### Cancellation Policy

| Kondisi | Additional Fee |
|---------|---------------|
| Cancel kapanpun | 0 IDR (booking fee 5.000 sudah hangus sebagai commitment) |
| No-show (> 1 jam tidak check-in) | 0 IDR + auto-expire (booking fee sudah hangus) |

### Reservation Rules

| Rule | Value |
|------|-------|
| Hold time | 1 jam setelah confirmation |
| Assignment modes | System-assigned atau User-selected |

---

## 14. Testing Strategy

### Unit Tests - `pkg/`

Target coverage: > 80% untuk business logic packages.

```
pkg/pricing/
  TestPricingEngine_BookingFee
  TestPricingEngine_HourlyRate_ExactHour
  TestPricingEngine_HourlyRate_StartedHour      ← 1j 1m = 2 jam
  TestPricingEngine_OvernightFee_CrossingMidnight
  TestPricingEngine_OvernightFee_SameDay
  TestPricingEngine_CancellationFee_Under2Min
  TestPricingEngine_CancellationFee_Over2Min
  TestPricingEngine_NoShow_NoExtraCharge
  TestPricingEngine_Overstay_NoPenalty

pkg/lock/
  TestRedisLock_AcquireSuccess
  TestRedisLock_AcquireFail_AlreadyLocked
  TestRedisLock_AutoExpiry

pkg/idempotency/
  TestIdempotency_FirstRequest_Processed
  TestIdempotency_DuplicateRequest_ReturnCached
```

### Integration Tests

```
Reservation → Billing flow (full DB transaction)
Payment webhook → Billing MarkPaid flow
Event publishing dan consuming via RabbitMQ
```

### End-to-End Scenarios (All Implemented & Passing)

| # | Skenario | Status |
|---|----------|--------|
| E2E-01 | Happy path reservation → check-in → check-out → pay | ✅ |
| E2E-02 | Double-book prevention - spot sama ditolak | ✅ |
| E2E-03 | User-selected spot contention - lock mechanism | ✅ |
| E2E-04 | Reservation expiry (no-show) - auto-expire, booking fee forfeited, no extra charge | ✅ |
| E2E-05 | Cancellation < 2 menit - fee 0 IDR | ✅ |
| E2E-06 | Cancellation > 2 menit - fee 5.000 IDR | ✅ |
| E2E-07 | Extended stay (overstay) - normal rate, no penalty | ✅ |
| E2E-08 | Overnight fee - crossing midnight +20.000 IDR | ✅ |
| E2E-09 | Payment QRIS - success | ✅ |
| E2E-10 | Payment QRIS - failure | ✅ |
| E2E-11 | Payment Virtual Account - VA number + settlement | ✅ |
| E2E-12 | Duplicate webhook - idempotent, no double-charge | ✅ |
| E2E-13 | Wrong-spot check-in — check-in rejected (spot mismatch) | ✅ |

---

## 15. Third-Party Libraries & Tools

| Library | Fungsi | Justifikasi |
|---------|--------|-------------|
| `google.golang.org/grpc` | gRPC framework | Required by soal |
| `google.golang.org/protobuf` | Protobuf serialization | Required by gRPC |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | gRPC ↔ REST transcoding | Single codebase untuk dua protokol |
| `github.com/rs/zerolog` | Structured logging | Zero allocation, JSON output |
| `github.com/spf13/viper` | Config management | Multi-source: env, file, remote |
| `github.com/redis/go-redis/v9` | Redis client | Official client, full-featured |
| `github.com/rabbitmq/amqp091-go` | RabbitMQ client | Official AMQP client |
| `github.com/jackc/pgx/v5` | PostgreSQL driver | Performa lebih baik dari lib/pq |
| `github.com/golang-migrate/migrate/v4` | DB migrations | Version control untuk schema |
| `github.com/sony/gobreaker` | Circuit breaker | Graceful degradation — wired di gateway gRPC client interceptor |
| `go.opentelemetry.io/otel` | Distributed tracing | Vendor-neutral, industry standard |
| `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` | gRPC OTel instrumentation | Auto-trace semua gRPC client calls |
| `firebase.google.com/go/v4` | FCM push notification | Official Firebase SDK |
| `github.com/aws/aws-sdk-go-v2` | AWS SDK (SES) | Official AWS SDK |
| `github.com/midtrans/midtrans-go` | Payment gateway | Official Midtrans Go SDK |
| `github.com/google/uuid` | UUID generation | Standard UUID v4 |
| `github.com/stretchr/testify` | Testing assertions | Standard di ekosistem Go |
| `github.com/testcontainers/testcontainers-go` | Integration testing | Real DB/Redis di test |

---

## 16. How to Run

### Prerequisites

- Go 1.25+
- Docker & Docker Compose
- `buf` CLI untuk proto generation
- `golang-migrate` CLI untuk database migration
- AWS CLI (untuk deployment ke AWS)
- Terraform >= 1.7 (untuk infrastructure provisioning)

### Quick Start (Local - Docker Compose)

```bash
# 1. Clone repository
git clone https://github.com/edysupardi/parkirpintar.git
cd parkirpintar

# 2. Copy environment config
cp .env.example .env
# Edit .env sesuai kebutuhan

# 3. Start all services (DB, Redis, RabbitMQ, 6 microservices)
docker compose up -d

# 4. Verify
curl http://localhost:8080/healthz
# → ok
```

### Quick Start (AWS Deployment)

```bash
# Prerequisites: AWS CLI configured, Docker running, Terraform installed

# 1. Deploy infrastructure + build + push + run
./scripts/deploy-infra.sh

# 2. Run database migrations
./scripts/run-migrations.sh up

# 3. Seed parking spots (400 spots)
./scripts/run-seed.sh

# 4. Verify
curl http://<ALB_DNS>/healthz
# → ok

# 5. Tear down (cost saving)
./scripts/destroy-infra.sh
```

### Run Tests

```bash
# Unit tests (pkg/)
go test ./pkg/... -v -race

# Integration tests (requires Docker - testcontainers)
go test -tags integration ./tests/integration/... -v

# E2E tests - all 12 scenarios (requires Docker - testcontainers)
go test -tags e2e ./tests/e2e/... -v

# Coverage report
go test ./pkg/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### API Documentation

Swagger/OpenAPI spec tersedia di:
```
GET /swagger.json
```

### Environment Variables

Lihat `.env.example` untuk daftar lengkap semua konfigurasi yang diperlukan.