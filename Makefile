.PHONY: build test web dev-api dev-web

build:
	mkdir -p bin
	go build -buildvcs=false -trimpath -o bin/clutchbase-api ./backend/cmd/api
	go build -buildvcs=false -trimpath -o bin/clutchbase-agent ./agent/cmd/agent

test:
	go test -race ./...
	go vet ./...
	npm run build --prefix web

web:
	npm ci --prefix web
	npm run build --prefix web

dev-api:
	set -a; . ./.env; set +a; go run -buildvcs=false ./backend/cmd/api

dev-web:
	npm run dev --prefix web
