# --- build stage ---
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/marketd ./cmd/marketd

# --- run stage ---
FROM alpine:3.20

RUN adduser -D -u 10001 appuser
USER appuser

COPY --from=build /out/marketd /usr/local/bin/marketd

EXPOSE 8080
ENTRYPOINT ["marketd"]
