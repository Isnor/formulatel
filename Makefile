
build: proto
	rm -rf out/ && mkdir out/
	CGO_ENABLED=0 go build -C formulatel -o ../out/ ./cmd/...

proto:
	@ which protoc protoc-gen-go || (echo "protoc and protoc-gen-go are required" && false)
	protoc -I=./protobuf --go_out=./formulatel ./protobuf/*.proto

k8s-cluster:
	@ which ctlptl || (echo "cattle patrol needed: https://github.com/tilt-dev/ctlptl" && false)
	ctlptl create cluster kind --name=kind-formulatel --registry=ctl-formulatel

migrate:
	migrate -path ./migrations -database postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable up

live-dashboard:
	curl -v -H 'Content-Type: application/json' --data @kubernetes/grafana-dashboard-live.json localhost:3000/api/dashboards/db