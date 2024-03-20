
build:
	rm -rf out/ && mkdir out/
	go build -C formulatel -o ../out/ ./...

run:
	go run main.go

proto:
	protoc -I=./protobuf --go_out=./formulatel --go-grpc_out=./formulatel ./protobuf/*