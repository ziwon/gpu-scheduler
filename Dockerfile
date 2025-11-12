# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.22
ARG CMD_PATH=cmd/scheduler

FROM golang:${GO_VERSION} AS build
ARG CMD_PATH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app "./${CMD_PATH}"

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/app /usr/bin/gpu-component
USER 65532:65532
ENTRYPOINT ["/usr/bin/gpu-component"]
