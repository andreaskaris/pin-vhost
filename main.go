package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/andreaskaris/pin-vhost/pkg/process"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
)

var (
	pinMode       = flag.String("pin-mode", "", "instruction for vCPU to pin to (accepted values: 'first', 'last', [0-9]+,)")
	discoveryMode = flag.Bool("discovery-mode", false, "discovery mode will print all discovered processes that match proc-name-filter")
)

func main() {
	// Parse provided parameters.
	flag.Parse()

	// Create new process instance (also validates that combination of discoveryMode and pinMode is valid).
	p, err := process.New(*discoveryMode, *pinMode, "^vhost-.*")
	if err != nil {
		log.Fatal(err)
	}

	// Subscribe to signals for terminating the program.
	stopper := make(chan os.Signal, 1)
	signal.Notify(stopper, os.Interrupt, syscall.SIGTERM)

	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
	}

	// Load pre-compiled programs and maps into the kernel.
	objs := kthreadCreateOnNodeObjects{}
	if err := loadKthreadCreateOnNodeObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	// Attach the pre-compiled program at fexit.. Each time the kernel function enters, the program
	// will increment the execution counter by 1. The read loop below polls this
	// map value once per second.
	//linkInstance, err := link.Kretprobe(kprobeFn, objs.KthreadCreateOnNode, nil)
	linkInstance, err := link.AttachTracing(link.TracingOptions{
		Program:    objs.KthreadCreateOnNode,
		AttachType: ebpf.AttachTraceFExit,
	})
	if err != nil {
		log.Fatalf("attaching eBPF tracing: %s", err)
	}
	defer linkInstance.Close()

	rd, err := perf.NewReader(objs.KthreadCreateOnNodeMap, os.Getpagesize())
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

	// Reconcile at startup for vhost threads that are already there.
	log.Printf(" == Reconciling on startup. Scanning directory %s for vhost processes.", p.GetProcDirectory())
	p.PinAll()

	// Whenever a kthread starting with vhost- is started, pin the process.
	log.Printf(" == Listening for vhost kthread creation.")
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
		// Record returns the PID in LittleEndian. Parse it as uint32 and then pin the process.
		// See kthread_create_on_node.c.
		pid := binary.LittleEndian.Uint32(record.RawSample)
		go func() {
			p.Pin(int(pid))
		}()
	}
}
