.PHONY: test plane agent admin db tidy

test:
	go test ./...

tidy:
	go mod tidy

db:
	docker compose up -d db

plane: db
	go run ./cmd/plane

agent:
	go run ./cmd/agent

admin:
	cd admin && npm run dev
