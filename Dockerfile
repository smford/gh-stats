FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod ./
# Copy go.sum if it exists
COPY go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/gh-stats ./cmd/gh-stats

FROM alpine:3.20

RUN apk add --no-cache git ca-certificates

COPY --from=builder /bin/gh-stats /bin/gh-stats

ENTRYPOINT ["/bin/gh-stats"]
