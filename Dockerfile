# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o web ./cmd/web

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/web .
COPY --from=builder /app/cmd/web/templates ./cmd/web/templates
COPY --from=builder /app/data/seed ./data/seed
RUN mkdir -p /app/data/avatars
EXPOSE 3000
CMD ["./web"]
