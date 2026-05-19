.PHONY: build test lint run tidy

build:
	go build ./...

test:
	go test ./... -race -count=1

lint:
	go vet ./...
	gofmt -l . | tee /dev/stderr | (! read)

run:
	go run ./cmd/skillcloud

tidy:
	go mod tidy
