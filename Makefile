VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all web build dev test vet image clean

all: web build

web:
	./web/build.sh

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/wackcluborchard-server ./cmd/wackcluborchard-server
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/wackcluborchard ./cmd/wackcluborchard
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/wackcluborchardctl ./cmd/wackcluborchardctl

# Simulated cluster, demo data, frontend served from disk so a web rebuild
# shows up on reload.
dev: web
	go run ./cmd/wackcluborchard-server -runtime sim -data ./data -web ./web/dist

test:
	go test ./...
	cd web && tsc -p tsconfig.json --noEmit

vet:
	go vet ./...
	gofmt -l cmd internal web/embed.go

image:
	docker build --build-arg VERSION=$(VERSION) -t ghcr.io/evanxdsouza/wackcluborchard:$(VERSION) .

clean:
	rm -rf bin web/dist/assets
