CONTAINER_IMAGE_LOCAL ?= localhost/pin-vhost
CONTAINER_IMAGE_REMOTE ?= quay.io/akaris/pin-vhost
CONTAINER_IMAGE ?= $(CONTAINER_IMAGE_LOCAL) 
CONTAINER_NAME ?= pin-vhost
PIN_MODE ?= first

.PHONY: deps
deps: # install dependencies
	yum install -y libbpf-devel clang llvm

.PHONY: generate
generate: # generate the cilium libraries
	go generate

.PHONY: generate-vmlinux-h
generate-vmlinux-h: # generate vmlinux.h (should not be necessary)
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

.PHONY: build
build: generate # build binary
	go build -buildvcs=false -o _output/pin-vhost

.PHONY: container-image
container-image: # build container image
	podman build -t localhost/pin-vhost .

.PHONY: push-container-image
push-container-image: # push container image to remote registry
	podman tag $(CONTAINER_IMAGE_LOCAL) $(CONTAINER_IMAGE_REMOTE)
	podman push $(CONTAINER_IMAGE_REMOTE)

.PHONY: run-container-foreground-discovery-mode
run-container-foreground-discovery-mode: # run container in foreground in discovery mode (no pinning)
	podman run --privileged -v /proc:/host/proc -v /sys:/host/sys \
	--pid=host --rm --name $(CONTAINER_NAME) -it $(CONTAINER_IMAGE) pin-vhost -discovery-mode -log-level 5

.PHONY: run-container-foreground
run-container-foreground: # run container in foreground
	podman run --privileged -v /proc:/host/proc -v /sys:/host/sys \
	--pid=host --rm --name $(CONTAINER_NAME) -it $(CONTAINER_IMAGE) pin-vhost -pin-mode $(PIN_MODE)  -log-level 5

.PHONY: run-container
run-container: # run container in background
	podman run --privileged -v /proc:/host/proc -v /sys:/host/sys \
	--pid=host -d --name $(CONTAINER_NAME) $(CONTAINER_IMAGE) pin-vhost -pin-mode $(PIN_MODE)  -log-level 5

.PHONY: stop-container
stop-container: # stop container running in background
	podman stop $(CONTAINER_NAME)
	podman rm $(CONTAINER_NAME)

.PHONY: deploy-daemonset
deploy-daemonset: # deploy the daemonset on kubernetes
	oc new-project pin-vhost || oc project pin-vhost
	oc adm policy add-scc-to-user privileged -z default || true
	kubectl apply -f config/daemonset.yaml

.PHONY: undeploy-daemonset
undeploy-daemonset:
	oc project default
	oc delete project pin-vhost

# https://dwmkerr.com/makefile-help-command/
.PHONY: help
help: # Show help for each of the Makefile recipes.
	@grep -E '^[a-zA-Z0-9 -]+:.*#'  Makefile | sort | while read -r l; do printf "\033[1;32m$$(echo $$l | cut -f 1 -d':')\033[00m:$$(echo $$l | cut -f 2- -d'#')\n"; done
