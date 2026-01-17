# Stage 1: Build frontend (platform-independent)
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend

WORKDIR /build
COPY ./web .
RUN npm install && npm run build


# Stage 2: Build Go binary (cross-compile)
FROM --platform=$BUILDPLATFORM golang:alpine AS backend

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=1.0.0

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /build/dist ./web/dist
RUN go build -ldflags "-s -w -X gpt-load/internal/version.Version=${VERSION}" -o gpt-load


# Stage 3: Final image (multi-platform)
FROM alpine

WORKDIR /app
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates

COPY --from=backend /build/gpt-load .
EXPOSE 3001
ENTRYPOINT ["/app/gpt-load"]
