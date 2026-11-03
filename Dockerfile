# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./src ./src

RUN go build -o part-tracker ./cmd/server

# Stage 2: Run
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /root/
COPY --from=builder /app/part-tracker .

EXPOSE 8080

CMD ["./part-tracker"]
