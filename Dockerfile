FROM golang:alpine AS builder

WORKDIR /flake
COPY . .
RUN go build -mod=vendor flake.go

FROM alpine:latest AS production
WORKDIR /flake
COPY --from=builder /flake/flake .

EXPOSE 10001
ENTRYPOINT ./flake -listen 0.0.0.0:10001 -etcdhosts http://${ETCD_SERVICE_HOST}:${ETCD_SERVICE_PORT}