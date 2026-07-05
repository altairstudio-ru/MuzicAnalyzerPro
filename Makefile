.PHONY: build run clean test fmt

build:
	go build -o bin/suno-archiver ./cmd/suno-archiver/

run:
	go run ./cmd/suno-archiver/

clean:
	rm -rf bin/ tmp/

test:
	go test ./...

fmt:
	go fmt ./...
