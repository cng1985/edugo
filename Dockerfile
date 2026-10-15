FROM node:22-bookworm AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend ./
ENV EDUGO_WEB_OUT=/out
RUN npm run build

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /out ./internal/web/dist
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags "-s -w" -o /out/edugo ./cmd/server

FROM debian:bookworm-slim
RUN useradd --create-home --uid 10001 edugo \
  && mkdir -p /data \
  && chown edugo:edugo /data
COPY --from=build /out/edugo /usr/local/bin/edugo
COPY configs/config.example.yaml /opt/edugo/config.example.yaml
USER edugo
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["edugo"]
