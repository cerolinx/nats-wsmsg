# syntax=docker/dockerfile:1

## Build
FROM golang:1.20.2-bullseye AS build

ADD . /app/

WORKDIR /app

RUN go mod download

RUN make build


## Deploy
FROM gcr.io/distroless/static-debian11

WORKDIR /

COPY --from=build /app/nats-wsmsg /usr/local/bin/nats-wsmsg

EXPOSE 8080

USER nonroot:nonroot

CMD ["nats-wsmsg", "websocket"]
