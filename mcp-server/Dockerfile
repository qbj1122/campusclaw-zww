# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcp-server ./cmd/mcp-server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
	&& addgroup -S mcp \
	&& adduser -S -G mcp mcp

WORKDIR /app
COPY --from=build /out/mcp-server /app/mcp-server

USER mcp
ENV MCP_PORT=8090
EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD wget -q -O /dev/null "http://127.0.0.1:${MCP_PORT}/health" || exit 1

ENTRYPOINT ["/app/mcp-server"]
