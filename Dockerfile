# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 works because modernc.org/sqlite is a pure-Go driver
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/bot ./cmd/bot

# ---- run stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/bot /app/bot
COPY config.example.yaml /app/config.example.yaml
VOLUME ["/app/data"]
EXPOSE 8080
ENTRYPOINT ["/app/bot"]
