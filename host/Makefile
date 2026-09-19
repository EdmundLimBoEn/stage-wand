.PHONY: build windows test arch-check
build:
	go build -buildvcs=false -o stagewand-host ./cmd/stagewand-host
windows:
	GOOS=windows GOARCH=amd64 go build -o stagewand-host.exe ./cmd/stagewand-host
test:
	go test ./internal/... ./cmd/... .
arch-check:
	bash arch/check.sh
