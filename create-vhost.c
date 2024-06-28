/*
Minimum viable program to create a vhost-... kthread for testing.
*/
#include <fcntl.h>
#include <sys/ioctl.h>
#include <unistd.h>

#define VHOST_VIRTIO 0xAF
#define VHOST_GET_FEATURES _IOR(VHOST_VIRTIO, 0x00, __u64)
#define VHOST_SET_FEATURES _IOW(VHOST_VIRTIO, 0x00, __u64)
#define VHOST_SET_OWNER _IO(VHOST_VIRTIO, 0x01)
#define VHOST_RESET_OWNER _IO(VHOST_VIRTIO, 0x02)
#define VHOST_SET_MEM_TABLE _IOW(VHOST_VIRTIO, 0x03, struct vhost_memory_kernel)
#define VHOST_SET_LOG_BASE _IOW(VHOST_VIRTIO, 0x04, __u64)

int main() {
    int fd = openat(AT_FDCWD, "/dev/vhost-net", O_RDWR);
    int res = ioctl(fd, VHOST_SET_OWNER, 0);
    sleep(120);
}