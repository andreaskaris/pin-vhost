package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/andreaskaris/pin-vhost/pkg/process"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"k8s.io/klog/v2"
)

var (
	pinMode       = flag.String("pin-mode", "", "instruction for vCPU to pin to (accepted values: 'first', 'last', [0-9]+,)")
	discoveryMode = flag.Bool("discovery-mode", false, "discovery mode will print all discovered processes that match proc-name-filter")
	logLevel      = flag.String("log-level", "0", "print higher klog-levels")
)

// getHostDirectory checks if /host/<dirname> exists, in that case we'll return that one, otherwise return just <dirname>
// (important for containerization).
func getHostDirectory(dirname string) string {
	hostDirname := path.Join("/host", dirname)
	if _, err := os.Stat(hostDirname); err == nil {
		dirname = hostDirname
	}
	return dirname
}

func main() {
	// Parse provided parameters.
	flag.Parse()

	var level klog.Level
	level.Set(*logLevel)

	// Create new process instance (also validates that combination of discoveryMode and pinMode is valid).
	p, err := process.New(*discoveryMode, *pinMode, "^vhost-.*", getHostDirectory("/proc"))
	if err != nil {
		klog.Fatal(err)
	}

	// Subscribe to signals for terminating the program.
	stopper := make(chan os.Signal, 1)
	signal.Notify(stopper, os.Interrupt, syscall.SIGTERM)

	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		klog.Fatal(err)
	}

	// Load pre-compiled programs and maps into the kernel.
	objs := kthreadCreateOnNodeObjects{}
	// With map pinning (needs change in kthread_create_on_node.c as well):
	//collectionOpts := &ebpf.CollectionOptions{Maps: ebpf.MapOptions{PinPath: getHostDirectory("/sys/fs/bpf/")}}
	// Without map pinning:
	collectionOpts := &ebpf.CollectionOptions{}
	if err := loadKthreadCreateOnNodeObjects(&objs, collectionOpts); err != nil {
		klog.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	// Attach the pre-compiled program at fexit.
	linkInstance, err := link.AttachTracing(link.TracingOptions{
		Program:    objs.KthreadCreateOnNode,
		AttachType: ebpf.AttachTraceFExit,
	})
	if err != nil {
		klog.Fatalf("attaching eBPF tracing: %s", err)
	}
	defer linkInstance.Close()

	// Create a reader for the map. The reader's rd.Read() will block until a new perf event is added to the map, see
	// below.
	rd, err := perf.NewReader(objs.KthreadCreateOnNodeMap, os.Getpagesize())
	if err != nil {
		klog.Fatalf("creating event reader: %s", err)
	}
	defer rd.Close()

	// Close the reader when the process receives a signal, which will exit
	// the read loop.
	go func() {
		<-stopper
		rd.Close()
	}()

	// Reconcile at startup for vhost threads that are already there.
	klog.Infof(" == Reconciling on startup. Scanning directory %s for vhost processes.", p.GetProcDirectory())
	if err := p.PinAll(); err != nil {
		klog.Fatal(err)
	}

	// Whenever a kthread starting with vhost- is started, pin the process.
	klog.Infof(" == Listening for vhost kthread creation.")
	for {
		record, err := rd.Read()
		klog.V(5).Info("BPF program detected vhost thread creation")
		if err != nil {
			if errors.Is(err, perf.ErrClosed) {
				klog.Info("Received signal, exiting..")
				return
			}
			klog.Infof("reading from reader: %s", err)
			continue
		}
		// Record returns the PID in LittleEndian. Parse it as uint32 and then pin the process.
		// See kthread_create_on_node.c.
		pid := binary.LittleEndian.Uint32(record.RawSample)
		klog.V(5).Infof("Received notification about vhost thread with PID %d", pid)
		go func() {
			if err := p.Pin(int(pid)); err != nil {
				klog.Warning(err)
			}
		}()
	}
}
