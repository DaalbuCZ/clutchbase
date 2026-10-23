FROM node:24-alpine AS web
WORKDIR /build/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS api
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY backend/ backend/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -o /clutchbase-api ./backend/cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /clutchbase-api /app/clutchbase-api
COPY --from=web /build/web/dist /app/web
ENV CLUTCHBASE_LISTEN=:8080 CLUTCHBASE_WEB_DIR=/app/web
EXPOSE 8080
ENTRYPOINT ["/app/clutchbase-api"]
