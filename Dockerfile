FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY . .
RUN go mod tidy
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/server ./cmd/server

FROM gcr.io/distroless/base-debian12

WORKDIR /app
COPY --from=builder /app/bin/server /app/server

EXPOSE 8080

ENTRYPOINT ["/app/server"]
