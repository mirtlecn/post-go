.PHONY: all build clean test smoke

BINARY := post-server
CMD := ./cmd/post-server

all: clean build

build:
	go build -o $(BINARY) $(CMD)

clean:
	rm -f $(BINARY)

test:
	go test ./...

smoke:
	./scripts/smoke_all.sh
