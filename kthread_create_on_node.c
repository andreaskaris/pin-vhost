//go:build ignore

#define __TARGET_ARCH_x86 true

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char __license[] SEC("license") = "Dual MIT/GPL";

// Set max_entries = 0 as this will set max_entries to the number of system CPUs.
// In bpf_perf_event_output, we hash to the CPU number, and if max_entries was too small we would drop events that occur
// on high CPU numbers. See https://github.com/cilium/ebpf/issues/209 for further details.
struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY); 
    __type(key, __u32);
    __type(value, __u32);
    __uint(max_entries, 0);
	// __uint(pinning, LIBBPF_PIN_BY_NAME); // Only if it turns out in the future that pinning was needed.
} kthread_create_on_node_map SEC(".maps"); 

static __always_inline int is_vhost_thread(struct task_struct *task)
{
	char vhost[] = "vhost-";
	int i;

	for (i = 0; i < sizeof(vhost) - 1; i++)
		if (vhost[i] != task->comm[i])
			return 0;

	return 1;
}

SEC("fexit/__kthread_create_on_node")
int BPF_PROG(kthread_create_on_node,
		int (*threadfn)(void *data),
		void *data,
		int node,
		const char namefmt[],
		va_list args,
		struct task_struct *task)
{
	if (!is_vhost_thread(task)) {
        return 0;
    }
    __u32 cpu = bpf_get_smp_processor_id(); // There's also BPF_F_CURRENT_CPU which serves the same purpose?
    unsigned int pid = task->pid;   
	bpf_perf_event_output(ctx, &kthread_create_on_node_map, cpu, &pid, sizeof(unsigned int));
	return 0;
}