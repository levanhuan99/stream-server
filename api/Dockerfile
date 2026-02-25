# ---- Build stage ----
FROM golang:1.23-alpine AS build

WORKDIR /app
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server .

# ---- Run stage ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget
COPY --from=build /server /server

EXPOSE 8080
ENTRYPOINT ["/server"]
