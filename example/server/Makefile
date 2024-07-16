export GOPROXY=https://goproxy.cn,direct

default: build

build: export GO111MODULE=on

build:
	rm -rf bin
	go get
	go build  -o server main.go
	go build  -o client ../client/main.go

clean:
	rm -rf bin
	rm server
