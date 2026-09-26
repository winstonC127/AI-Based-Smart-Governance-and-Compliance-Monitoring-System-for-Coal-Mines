# ---- Build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o coal-governance-backend .

# ---- Runtime stage ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
RUN mkdir -p /app/uploads
COPY --from=builder /app/coal-governance-backend .
EXPOSE 8080
CMD ["./coal-governance-backend"]
