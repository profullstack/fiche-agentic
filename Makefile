# fiche-agentic — Go build. (Upstream's C Makefile lives in legacy/.)
BIN=fiche-agentic
prefix=/usr/local/bin

all: build

build:
	go build -o $(BIN) ./cmd/fiche-agentic

run: build
	./$(BIN)

test:
	go test ./...

vet:
	go vet ./...
	gofmt -l .

install: build
	install -m 0755 $(BIN) $(prefix)

clean:
	rm -f $(BIN)

.PHONY: all build run test vet install clean
