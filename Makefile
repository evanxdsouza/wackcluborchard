VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all web build dev test vet image clean

all: web build

web:
	./web/build.sh

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/orchard-server ./cmd/orchard-server
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/orchard ./cmd/orchard
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/orchardctl ./cmd/orchardctl

# Simulated cluster, demo data, frontend served from disk so a web rebuild
# shows up on reload.
dev: web
	go run ./cmd/orchard-server -runtime sim -data ./data -web ./web/dist

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
