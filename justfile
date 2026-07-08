set positional-arguments

default:
	@just --list

build:
	go build .

clean:
	rm -rf bin coverage.out

download:
	go mod download


tidy:
	go mod tidy

fmt:
	gofmt -w -s ./.

fmt-check:
	@files=$(gofmt -l -s ./.); if [ -n "$files" ]; then printf '%s\n' "$files"; exit 1; fi

mod-check:
	go mod tidy -diff

lint: fmt-check
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...

verify: download fmt-check mod-check
	golangci-lint run

hooks:
	lefthook install
