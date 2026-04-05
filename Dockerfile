FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ENV CGO_ENABLED=1
RUN go build -ldflags="-s -w" -o /helm .

FROM alpine:latest

RUN apk add --no-cache sqlite-libs ca-certificates

COPY --from=builder /helm /usr/local/bin/helm

RUN mkdir -p /root/.config/helm

VOLUME ["/root/.config/helm"]

ENTRYPOINT ["helm"]
