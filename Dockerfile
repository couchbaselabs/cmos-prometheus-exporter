FROM golang:1.17 AS builder

WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download

COPY . /src/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ./yacpe -tags netgo ./cmd/yacpe

FROM alpine

COPY --from=builder --chmod=0777 /src/yacpe /yacpe

ENTRYPOINT ["/yacpe"]
