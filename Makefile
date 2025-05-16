dev:
	@go build -ldflags="-s -w" -o _dist/standalone ./cmd/standalone && cd cmd/standalone && docker compose up --build

build:
	@go build -ldflags="-s -w" -o _dist/standalone ./cmd/standalone

test:
	@go test ./internal -v -count=1 -race

loadtest:
	@cd cmd/loadtest && go run *.go

parity:
	@cd cmd/loadtest && go run *.go -parity

parity-target:
	@cd cmd/loadtest && docker compose up --build

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
