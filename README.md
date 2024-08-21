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

Make sure that the system has hugepages:

```
# cat <<'EOF' > /etc/sysctl.d/80-hugepages.conf
# Number of 2MB hugepages desired
vm.nr_hugepages=1024
EOF
# reboot
```

After reboot:

```
# grep -iE 'HugePages_Free|Hugepagesize' /proc/meminfo
HugePages_Free:     1024
Hugepagesize:       2048 kB
```

Run the application, e.g.:

```
# make run-container-foreground CONTAINER_IMAGE=quay.io/akaris/pin-vhost PIN_MODE=last
```
> **Note:** If the vhost-net module has not been loaded before, run: `modprobe vhost-net`. dpdk-testpmd will autoload it,
so you could also run that one first. However, for a valid test case you want to start the pin-vhost first, and
dpdk-testpmd second.

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

## Deploying on OpenShift

You can deploy the DaemonSet in its own namespace with:

```
make deploy-daemonset
```

You can remove the DaemonSet and all related resources with:

```
make undeploy-daemonset
```

You can check logs with:

```
$ oc logs -n pin-vhost -l name=pin-vhost
I0821 18:25:18.297248  707069 main.go:96]  == Reconciling on startup. Scanning directory /host/proc for vhost processes.
I0821 18:25:18.323208  707069 process.go:179] Pinning pid 706887. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:25:18.323252  707069 process.go:179] Pinning pid 706888. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:25:18.323280  707069 process.go:179] Pinning pid 706889. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:25:18.323307  707069 process.go:179] Pinning pid 706890. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:25:18.327496  707069 main.go:102]  == Listening for vhost kthread creation.
I0821 18:26:36.704901  707069 process.go:179] Pinning pid 709298. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:26:36.705299  707069 process.go:179] Pinning pid 709299. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:26:36.705823  707069 process.go:179] Pinning pid 709300. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
I0821 18:26:36.706329  707069 process.go:179] Pinning pid 709301. Configured pin-mode: "first". Currrent cpus_allowed_list: "28,30,32,34,36,38,40,42,84,86,88,90,92,94,96,98". New CPU set: "28"
```