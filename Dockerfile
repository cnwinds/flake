FROM golang:alpine AS builder

WORKDIR /flake
COPY . .
RUN go build -mod=vendor flake.go

FROM alpine:latest AS production
WORKDIR /uuid_service
COPY --from=builder /flake/flake .
#HEALTHCHECK --interval=5s --timeout=3s \
#	CMD curl -fs http://localhost/ || exit 1
EXPOSE 10001
ENTRYPOINT ["./flake"]