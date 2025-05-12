dev:
	@go build -ldflags="-s -w" -o _dist/standalone ./cmd/standalone && cd cmd/standalone && docker compose up --build

build:
	@go build -ldflags="-s -w" -o _dist/standalone ./cmd/standalone

test:
	@go test

parity:
	@PARITY_CHECK=true go test -v

bench:
	@go test -bench=. -run=_ -v

unit:
	@go test ./... -tags unit -v

cover:
	@mkdir -p _dist
	@go test ./internal -coverprofile=_dist/coverage.out -v
	@go tool cover -html=_dist/coverage.out -o _dist/coverage.html

cloc:
	@cloc . --exclude-dir=.git,_example,_dist
