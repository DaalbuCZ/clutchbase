.PHONY: build test access-plugin web dev-api dev-web

build:
	mkdir -p bin
	go build -buildvcs=false -trimpath -o bin/clutchbase-api ./backend/cmd/api
	go build -buildvcs=false -trimpath -o bin/clutchbase-agent ./agent/cmd/agent
	go build -buildvcs=false -trimpath -o bin/clutchbase-host ./agent/cmd/host
	go build -buildvcs=false -trimpath -o bin/clutchbase-guard ./agent/cmd/guard

access-plugin:
	dotnet build plugins/ClutchbaseAccess -c Release -p:CounterStrikeSharpDir="$(CSSHARP_DIR)"
	dotnet run --project plugins/ClutchbaseAccess.Tests -c Release

test:
	go test -race ./...
	go vet ./...
	npm run test --prefix web
	npm run build --prefix web

web:
	npm ci --prefix web
	npm run build --prefix web

dev-api:
	set -a; . ./.env; set +a; go run -buildvcs=false ./backend/cmd/api

dev-web:
	npm run dev --prefix web
