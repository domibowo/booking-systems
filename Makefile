.PHONY: run build test tidy db-up db-down db-logs swagger

run: swagger
	go run ./cmd/api

build: swagger
	go build -o bin/api ./cmd/api

# Regenerates docs/ from the @-annotations on handlers. Uses `go run` (not a
# globally installed swag binary) so the tool version stays pinned to go.mod.
swagger:
	go run github.com/swaggo/swag/cmd/swag init -g cmd/api/main.go --output docs --parseDependency --parseInternal

test:
	go test ./...

tidy:
	go mod tidy

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres
