## Build instructions

### Containerized

Build image locally:

```
make container-image
```

Run image in foreground:

```
make run-container-foreground
# Or, with a pre-built image from my quay repository:
#     make run-container-foreground CONTAINER_IMAGE=quay.io/akaris/pin-vhost
# You can also change the pin-mode:
#     make run-container-foreground CONTAINER_IMAGE=quay.io/akaris/pin-vhost PIN_MODE=last
```

Run image in foreground in discovery mode (no pinning):

```
make run-container-foreground-discovery-mode
```

Run image in background:

```
make run-container
```

Remove container running in background:

```
make stop-container
```

## Running a DPDK app with vhost- threads for testing

I tested this on a RHEL 9 system with dpdk-testpmd.

First, run the application, e.g.:

```
# make run-container-foreground CONTAINER_IMAGE=quay.io/akaris/pin-vhost PIN_MODE=last
```

Then, run testpmd:

```
# taskset -c 0-7 /usr/bin/dpdk-testpmd -l 0-7 -m2048 --file-prefix=0 -a 0000:07:00.0  --vdev=virtio_user0,path=/dev/vhost-net,queue_size=1024,iface=vf0  -- -i --nb-cores=2 --cmdline-file=/root/commands.txt --portmask=f --rxq=1 --txq=1 --forward-mode=io
```

With:

```
# cat /root/commands.txt 
set portlist 0,1
show config fwd
show port info all
show port stats all
start
```

You should see the following output:

```
# make run-container-foreground CONTAINER_IMAGE=quay.io/akaris/pin-vhost PIN_MODE=last
podman run --privileged -v /proc:/proc --pid=host --rm --name pin-vhost -it quay.io/akaris/pin-vhost pin-vhost -pin-mode last
2024/08/14 14:03:03 Started with parameters: procDirectory: /proc/
2024/08/14 14:03:03 Waiting for events..
2024/08/14 14:03:03 Scanning directory /proc/ for vhost processes
2024/08/14 14:03:15 Scanning directory /proc/ for vhost processes
2024/08/14 14:03:15 Pinning pid 15237 with pin-mode "last" and current cpus_allowed_list 0-7 to CPU set 7 (mask [128 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0])
```

And you can verify with taskset:

```
# taskset -c -p $(ps aux | pgrep vhost-)
```