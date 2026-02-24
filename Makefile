.PHONY: build run test clean

BINARY := miniclaw

build:
	go build -o $(BINARY) .

run:
	go run . $(filter-out $@,$(MAKECMDGOALS))

test:
	go test ./...

clean:
	rm -f $(BINARY)

%:
	@:
