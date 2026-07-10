BINARY := seedstrem
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build web-build test lint vet fmt run docker clean

build: web-build
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/seedstrem

go-build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/seedstrem

web-build:
	cd web && npm run build

test:
	go test -race -cover ./...

vet:
	go vet ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

run:
	go run ./cmd/seedstrem --config ./config.yaml

docker:
	docker build -t seedstrem:latest .

clean:
	rm -rf bin coverage.out
