FROM registry.fedoraproject.org/fedora-minimal:latest
WORKDIR /app
COPY . .
RUN microdnf -y install make golang git libbpf-devel clang llvm && microdnf clean all
RUN make generate
RUN make build
