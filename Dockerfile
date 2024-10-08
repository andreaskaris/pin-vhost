FROM registry.fedoraproject.org/fedora-minimal:latest AS builder
WORKDIR /app
COPY . .
RUN microdnf -y install make golang git libbpf-devel clang llvm && microdnf clean all
RUN make generate
RUN make build

FROM registry.fedoraproject.org/fedora-minimal:latest
COPY --from=builder /app/_output/* /usr/local/bin/
# RUN microdnf -y install procps-ng -y && microdnf clean all
# RUN microdnf -y install make golang git libbpf-devel clang llvm bpftool && microdnf clean all