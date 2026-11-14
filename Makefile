BIN_DIR := build

.PHONY: all build-poller-windows build-poller-linux build-ingest-linux up down logs

all: build-ingest-linux build-poller-windows

build-poller-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/poller.exe ./cmd/poller

build-poller-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/poller-linux ./cmd/poller

# ingest собираем ДЛЯ LINUX (он живёт в контейнере), вне зависимости от ОС хоста
build-ingest-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/ingest-linux ./cmd/ingest

build-web-linux:
	cd web && (test -d node_modules || npm ci) && npm run build
	cd .. && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/web-linux ./cmd/web

# `up` сначала собирает ingest, потом поднимает compose
up: build-ingest-linux
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f ingest
