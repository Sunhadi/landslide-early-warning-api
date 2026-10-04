.PHONY: run build test vet tidy docker clean

run:
	go run ./cmd/api

build:
	go build -o bin/api.exe ./cmd/api

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

docker:
	docker compose up --build

clean:
	rm -rf bin
