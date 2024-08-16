//go:build ignore

#define __TARGET_ARCH_x86 true

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char __license[] SEC("license") = "Dual MIT/GPL";

struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY); 
    __type(key, __u32);
    __type(value, __u32);
    __uint(max_entries, 16);
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
    __u32 cpu = bpf_get_smp_processor_id();
    unsigned int pid = task->pid;   
	bpf_perf_event_output(ctx, &kthread_create_on_node_map, cpu, &pid, sizeof(unsigned int));
	return 0;
}