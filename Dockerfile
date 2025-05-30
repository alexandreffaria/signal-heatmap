# Stage 1: build the Go binary
FROM golang:1.24-alpine AS build

WORKDIR /app
# cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# copy the rest of your code
COPY . .

# compile
RUN go build -o server .

# Stage 2: create a small runtime image
FROM alpine:latest

# needed for TLS (Firestore client)
RUN apk add --no-cache ca-certificates

WORKDIR /root/

# copy the server binary and your static assets & data.json
COPY --from=build /app/server .
COPY --from=build /app/static ./static
COPY --from=build /app/data.json .

# expose the port your server listens on
EXPOSE 8082

# run it
CMD ["./server"]
