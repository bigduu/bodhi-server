FROM node:22-alpine AS web

WORKDIR /web
COPY cmd/server/web/package.json cmd/server/web/package-lock.json ./
RUN npm ci
COPY cmd/server/web/ ./
RUN npm run build

FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /web/dist ./cmd/server/web/dist
RUN CGO_ENABLED=0 go build -o /bodhi-server ./cmd/server

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /bodhi-server /usr/local/bin/bodhi-server

EXPOSE 8080
ENTRYPOINT ["bodhi-server"]
