.PHONY: deps
deps:
	yum install -y libbpf-devel clang llvm

.PHONY: generate
generate:
	go generate

.PHONY: build
build: generate
	go build -buildvcs=false -o _output/pin-vhost

.PHONY: container-image
container-image:
	podman build -t localhost/pin-vhost .
