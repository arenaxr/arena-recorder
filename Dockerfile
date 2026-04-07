FROM golang:1.26-alpine AS builder

WORKDIR /app

# Download Go modules
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o arena-recorder .

# Final minimal execution container
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/arena-recorder .

EXPOSE 8885

CMD ["./arena-recorder"]
