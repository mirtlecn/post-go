.PHONY: all build clean test smoke run assets-sync assets-sync-optional

BINARY := post-server
CMD := ./cmd/post-server

all: build

build: clean
	go build -o $(BINARY) $(CMD)

clean:
	rm -f $(BINARY)

test: build
	go test ./...

run: clean
	go run $(CMD)

smoke: build
	./scripts/smoke_all.sh

assets-sync:
	go run ./scripts/update_embedded_assets.go