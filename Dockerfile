FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/proxy ./cmd

FROM alpine:3.19
COPY --from=builder /out/proxy /usr/local/bin/proxy

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/proxy"]
