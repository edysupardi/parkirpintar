FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/seed ./scripts/seed

FROM scratch
COPY --from=builder /bin/seed /seed
ENTRYPOINT ["/seed"]
