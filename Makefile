
build: proto
	rm -rf out/ && mkdir out/
	CGO_ENABLED=0 go build -C formulatel -o ../out/ ./cmd/...

proto:
	@ which protoc protoc-gen-go || (echo "protoc and protoc-gen-go are required" && false)
	rm formulatel/internal/genproto/telemetry.pb.go
	protoc -I=./protobuf --go_out=./formulatel ./protobuf/*.proto

coverage:
	go -C formulatel test -coverprofile=coverage.out -covermode=atomic -race ./f123 ./internal/timescale
	go -C formulatel tool cover -html=coverage.out -o coverage.html

# TODO: we have a cluster.yml now, need to reconcile this with that file
k8s-cluster:
	@ which ctlptl || (echo "cattle patrol needed: https://github.com/tilt-dev/ctlptl" && false)
	ctlptl create cluster kind --name=kind-formulatel --registry=ctl-formulatel

migrate:
	migrate -path ./migrations -database postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable up

live-dashboard:
	curl -v -H 'Content-Type: application/json' --data @kubernetes/config/dashboards/dashboard-live.json localhost:3000/api/dashboards/db

static-dashboard:
	curl -v -H 'Content-Type: application/json' --data @kubernetes/config/dashboards/dashboard-static.json localhost:3000/api/dashboards/db
