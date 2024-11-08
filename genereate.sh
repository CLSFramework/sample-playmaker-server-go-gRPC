export PATH=$PATH:$(go env GOPATH)/bin

protoc -I ./idl \
  --go_out=./service_pb \
  --go-grpc_out=./service_pb \
  ./idl/service.proto
