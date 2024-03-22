FROM golang:1.22.1 AS build

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

FROM golang:1.22.1

WORKDIR /out
COPY --from=build /src/out ./