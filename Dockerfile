FROM golang:1.22.1 AS build

# TODO: turns out this isn't a good idea, do the correct thing instead with go tools/go generate
WORKDIR /protoc
RUN apt update && apt install -y unzip && curl -o protoc.zip -L https://github.com/protocolbuffers/protobuf/releases/download/v25.1/protoc-25.1-linux-x86_64.zip
RUN unzip protoc.zip -d /usr/

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

WORKDIR /src

COPY protobuf ./protobuf
COPY formulatel ./formulatel
COPY Makefile .

RUN make proto
RUN make build

FROM gcr.io/distroless/static-debian11:debug

COPY --from=build /src/out/rpc /

ENTRYPOINT ["/rpc"]