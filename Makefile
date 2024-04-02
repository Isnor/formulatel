
build: proto
	rm -rf out/ && mkdir out/
	CGO_ENABLED=0 go build -C formulatel -o ../out/ ./...

proto:
	protoc -I=./protobuf --go_out=./formulatel --go-grpc_out=./formulatel ./protobuf/*.proto protobuf/*/*.proto

k8s-cluster:
	@ which ctlptl || (echo "cattle patrol needed: https://github.com/tilt-dev/ctlptl" && false)
	ctlptl create cluster kind --name=kind-formulatel --registry=ctl-formulatel