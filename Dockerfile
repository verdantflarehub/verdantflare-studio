# Gin/Vue website build; all build steps use scripts/build.sh.
FROM node:24-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/ ./
COPY scripts/build.sh /src/scripts/build.sh
RUN bash /src/scripts/build.sh frontend

FROM golang:1.26-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
COPY scripts/build.sh scripts/build.sh
RUN go mod download
COPY cmd/web/ cmd/web/
COPY internal/ internal/
COPY frontend/assets.go frontend/assets.go
COPY --from=frontend /src/frontend/dist/ frontend/dist/
RUN bash scripts/build.sh web --backend-only

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /src/build/studio-web /studio-web
ENV STUDIO_LISTEN=0.0.0.0:8000
EXPOSE 8000
ENTRYPOINT ["/studio-web"]
