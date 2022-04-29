MAKEFLAGS += --silent
GO_MISSING_HELP = "\033[0;31mIMPORTANT\033[0m: Couldn't find go. Please install it first.\033[0m\n"

GO := $(shell command -v go 2> /dev/null)

all: clean format test build

## help: Prints a list of available build targets.
help:
	echo "Usage: make <OPTIONS> ... <TARGETS>"
	echo ""
	echo "Available targets are:"
	echo ''
	sed -n 's/^##//p' ${PWD}/Makefile | column -t -s ':' | sed -e 's/^/ /'
	echo
	echo "Targets run by default are: `sed -n 's/^all: //p' ./Makefile | sed -e 's/ /, /g' | sed -e 's/\(.*\), /\1, and /'`"

## clean: Removes any previously created build artifacts.
clean:
	rm -f ./k6

## check-prereq: Checks that required sofware is installed
check-prereq:
ifndef GO
	printf $(GO_MISSING_HELP)
	exit 1
endif

## build: Builds a custom 'k6' with the local extension. 
build: check-prereq
	go install go.k6.io/xk6/cmd/xk6@latest
	xk6 build --with github.com/elastic/xk6-output-elasticsearch=.

## format: Applies Go formatting to code.
format:
	go fmt ./...

## test: Executes any unit tests.
test:
	go test -cover -race ./...

.PHONY: build clean check-prereq format help test
