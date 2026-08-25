# Build stage
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /rpi-disk-shelf-controller ./cmd/rpi-disk-shelf-controller/main.go
# RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -o /rpi-disk-shelf-controller ./cmd/rpi-disk-shelf-controller

# Final stage
FROM alpine:latest

RUN apk --no-cache add bash

WORKDIR /

COPY --from=builder /rpi-disk-shelf-controller /rpi-disk-shelf-controller

ENTRYPOINT ["/rpi-disk-shelf-controller"]
