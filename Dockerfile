FROM golang:1.17 AS builder

WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download

COPY . /src/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ./cmos-exporter -tags netgo ./cmd/cmos-exporter

FROM alpine
RUN apk add bash

COPY --from=builder /src/cmos-exporter /usr/bin/cmos-exporter

RUN chmod 755 /usr/bin/cmos-exporter

ENTRYPOINT ["/usr/bin/cmos-exporter"]
