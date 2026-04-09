.PHONY: run test docker-up docker-down

run:
	go run ./cmd/server

test:
	go test ./...

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v
