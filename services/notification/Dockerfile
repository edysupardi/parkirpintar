FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/notification ./services/notification/cmd

FROM scratch
COPY --from=builder /bin/notification /notification
ENTRYPOINT ["/notification"]
