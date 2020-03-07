FROM golang:alpine AS build
LABEL maintainer="cnwinds@163.com"
WORKDIR /flake
COPY . .
RUN go build -mod=vendor flake.go

FROM alpine:latest AS production
WORKDIR /flake
COPY --from=build /flake/flake .

EXPOSE 10001
ENTRYPOINT ./flake -listen 0.0.0.0:10001 -etcdhosts http://etcd:2379