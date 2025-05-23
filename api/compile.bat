protoc --proto_path=. \
  --go_out=paths=source_relative:. \
  --go-grpc_out=paths=source_relative:. \
  api/uuid.proto