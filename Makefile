BINARY := aienv
PREFIX := /usr/local

.PHONY: build install test clean

build:
	go build -o $(BINARY) ./cmd/aienv

install: build
	sudo install -m 755 $(BINARY) $(PREFIX)/bin/$(BINARY)

test:
	go test ./...

clean:
	rm -f $(BINARY)
