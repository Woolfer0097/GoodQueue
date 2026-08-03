FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/goodqueue-backend ./cmd/goodqueue-backend && \
    GOBIN=/out go install github.com/pressly/goose/v3/cmd/goose

FROM alpine:3.23
RUN apk add --no-cache ca-certificates wget && addgroup -S goodqueue && adduser -S -G goodqueue goodqueue
WORKDIR /app
COPY --from=builder /out/goodqueue-backend /usr/local/bin/goodqueue-backend
COPY --from=builder /out/goose /usr/local/bin/goose
COPY migrations ./migrations
USER goodqueue
EXPOSE 8080
ENTRYPOINT ["goodqueue-backend"]
