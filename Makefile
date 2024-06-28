.PHONY: build
build:
	go build -o _output/pin-vhost

.PHONY: build-vhost
build-vhost:
	gcc -o _output/create-vhost create-vhost.c