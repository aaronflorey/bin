set positional-arguments

default:
	@just --list

build:
	go build .

clean:
	rm -rf bin coverage.out

download:
	go mod download
	go mod tidy

fmt:
	gofmt -w -s ./.

lint:
	go fmt ./...
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...

verify: download fmt
	golangci-lint run

hooks:
	lefthook install
