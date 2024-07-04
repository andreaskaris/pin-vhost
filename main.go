// This program demonstrates attaching an eBPF program to a kernel symbol.
// The eBPF program will be attached to the start of the sys_execve
// kernel function and prints out the number of times it has been called
// every second.
package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
)

// Constants for eBPF part.
const (
	kprobeFn        = "vhost_net_ioctl" // Name of the kernel function to trace.
	VHOST_VIRTIO    = 175
	VHOST_SET_OWNER = 1 // https://github.com/torvalds/linux/blob/master/include/uapi/linux/vhost.h#L32
)

var (
	procDirectory = "/proc/"
	pinMode       = flag.String("pin-mode", "", "instruction for vCPU to pin to (accepted values: 'first', 'last', [0-9]+,)")
	discoveryMode = flag.Bool("discovery-mode", false, "discovery mode will print all discovered processes that match proc-name-filter")
)

func main() {
	// Parse provided parameters.
	flag.Parse()
	if !*discoveryMode {
		if !pinModeRegex.MatchString(*pinMode) {
			log.Fatal("Must provide a valid pin-mode when discovery mode is off")
		}
	} else {
		if *pinMode != "" {
			log.Fatal("Cannot provide a pin-mode in discovery-mode")
		}
	}

	// Check if /host/proc exists, in that case we'll be looking there for process information (important for containerization).
	hostProc := path.Join("/host", procDirectory)
	if _, err := os.Stat(hostProc); err == nil {
		procDirectory = hostProc
	}

	// Print a status message at start.
	log.Printf("Started with parameters: procDirectory: %s", procDirectory)

	// Subscribe to signals for terminating the program.
	stopper := make(chan os.Signal, 1)
	signal.Notify(stopper, os.Interrupt, syscall.SIGTERM)

	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
	}

	// Load pre-compiled programs and maps into the kernel.
	objs := kprobeVhostNetIoctlObjects{}
	if err := loadKprobeVhostNetIoctlObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	// Open a Kprobe at the entry point of the kernel function and attach the
	// pre-compiled program. Each time the kernel function enters, the program
	// will increment the execution counter by 1. The read loop below polls this
	// map value once per second.
	kp, err := link.Kprobe(kprobeFn, objs.KprobeVhostNetIoctl, nil)
	if err != nil {
		log.Fatalf("opening kprobe: %s", err)
	}
	defer kp.Close()

	log.Println("Waiting for events..")
	rd, err := perf.NewReader(objs.KprobeVhostNetIoctlMap, os.Getpagesize())
	if err != nil {
		log.Fatalf("creating event reader: %s", err)
	}
	defer rd.Close()

	// Close the reader when the process receives a signal, which will exit
	// the read loop.
	go func() {
		<-stopper
		rd.Close()
	}()

	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				log.Println("Received signal, exiting..")
				return
			}
			log.Printf("reading from reader: %s", err)
			continue
		}
		// log.Printf("Record: %+v (record.RawSample[0]: 0x%x)", record, record.RawSample[0])
		if uint8(record.RawSample[1]) == VHOST_VIRTIO && uint8(record.RawSample[0]) == VHOST_SET_OWNER {
			//log.Println("VHOST_SET_OWNER called")
			go func() {
				time.Sleep(2 * time.Second)
				scanProc(discoveryMode, pinMode)
			}()
		}
	}
}
