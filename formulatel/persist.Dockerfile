FROM FROM --platform=$BUILDPLATFORM golang:1.26.4 AS build

WORKDIR /src

COPY . .

ARG TARGETOS
ARG TARGETARCH

# this was for OBI; I never got OBI working properly, so I don't know if these were ever needed.
# they were an AI suggestion, the root cause for which I did not grok
# -gcflags="all=-N -l" -ldflags="-extldflags=-static" # add these build flags to `go build` to produce an
# unoptimized binary without inlining or ELF optimization.
RUN mkdir out && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o ./out/persist ./cmd/persist

# the `debug` tag provides an image with a shell for us to exec into when the container is running
FROM gcr.io/distroless/static-debian13:debug

COPY --from=build /src/out/persist /

ENTRYPOINT ["/persist"]