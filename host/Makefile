.PHONY: build windows test arch-check
build:
	go build -o stagewand-host ./cmd/stagewand-host
windows:
	GOOS=windows GOARCH=amd64 go build -o stagewand-host.exe ./cmd/stagewand-host
test:
	go test ./...
arch-check:
	bash arch/check.sh
