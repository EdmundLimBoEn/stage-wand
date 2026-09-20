.PHONY: build windows test arch-check debian-check deb
build:
	go build -buildvcs=false -o stagewand-host ./cmd/stagewand-host
windows:
	GOOS=windows GOARCH=amd64 go build -o stagewand-host.exe ./cmd/stagewand-host
test:
	go test ./internal/... ./cmd/... .
arch-check:
	bash arch/check.sh
debian-check:
	bash debian/check.sh
deb:
	bash debian/build.sh

.PHONY: desktop
desktop: build
	cmake -S desktop -B desktop/build -DCMAKE_BUILD_TYPE=Release
	cmake --build desktop/build
	cp stagewand-host desktop/build/stagewand-host.tmp
	mv -f desktop/build/stagewand-host.tmp desktop/build/stagewand-host
