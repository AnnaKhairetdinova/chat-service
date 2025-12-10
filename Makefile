migrate:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations down

status:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations status
