# Multi-stage static build for Factura (API + embedded SPA).
# Frontend is pre-built into pkg/web/dist (committed or CI-synced); no npm in the image.
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=
RUN ver="${VERSION#v}"; \
  if [ -z "$ver" ]; then ver="$(cat VERSION 2>/dev/null || echo 0.0.0)"; fi; \
  CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$ver" \
  -o /factura ./cmd/factura

FROM scratch
COPY --from=build /factura /factura
ENV FACTURA_DB=/data/factura.db
ENV FACTURA_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/factura", "serve", "--addr", ":8080"]
