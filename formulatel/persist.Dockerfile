# TODO: use latest major version?
FROM golang:1.25.1 AS build

WORKDIR /src

COPY . .

# -gcflags="all=-N -l" -ldflags="-extldflags=-static" # add these build flags to `go build` to produce an
# unoptimized binary without inlining or ELF optimization.
RUN mkdir out && CGO_ENABLED=0 GOOS=linux go build -o ./out/persist ./cmd/persist

FROM gcr.io/distroless/static-debian13:debug

COPY --from=build /src/out/persist /

ENTRYPOINT ["/persist"]