.PHONY: build run clean test fmt setup-python

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
PYTHON := .venv/bin/python3
PIP := .venv/bin/pip

LDFLAGS := -ldflags="-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/suno-archiver ./cmd/suno-archiver/

run:
	go run ./cmd/suno-archiver/

clean:
	rm -rf bin/ tmp/
	rm -rf .venv/

test:
	go test ./...

fmt:
	go fmt ./...

setup-python:
	python3 -m venv .venv
	$(PIP) install --upgrade pip setuptools wheel
	$(PIP) install -r analyzer/requirements.txt
