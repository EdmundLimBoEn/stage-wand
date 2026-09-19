.PHONY: build windows test
build:
	go build -o stagewand-host ./cmd/stagewand-host
windows:
	GOOS=windows GOARCH=amd64 go build -o stagewand-host.exe ./cmd/stagewand-host
test:
	go test ./...
