FROM golang:1.26.4-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/finance-api ./cmd/api

FROM alpine:3.22

# Run as a non-root, unprivileged user. The binary needs no elevated
# capability (8080 is an unprivileged port, no host paths are touched), so
# there is no reason for a compromised process to have root inside the
# container.
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/finance-api /app/finance-api
USER app

EXPOSE 8080
ENTRYPOINT ["/app/finance-api"]
