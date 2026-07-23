FROM golang:1.26.4-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/finance-api ./cmd/api

FROM alpine:3.22

WORKDIR /app
COPY --from=build /out/finance-api /app/finance-api

EXPOSE 8080
ENTRYPOINT ["/app/finance-api"]
