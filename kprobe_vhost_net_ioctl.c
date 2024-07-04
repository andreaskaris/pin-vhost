//go:build ignore

#define __TARGET_ARCH_x86 true

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>
#include "common.h"

char __license[] SEC("license") = "Dual MIT/GPL";

struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY); 
    __type(key, __u32);
    __type(value, __u32);
    __uint(max_entries, 16);
} kprobe_vhost_net_ioctl_map SEC(".maps"); 

SEC("kprobe/vhost_net_ioctl")
int kprobe_vhost_net_ioctl(struct pt_regs *ctx) {
    __u32 cpu = bpf_get_smp_processor_id();
	unsigned int ioctl = (unsigned int) ctx->rsi;
	//unsigned int ioctl = 123;
	bpf_perf_event_output(ctx, &kprobe_vhost_net_ioctl_map, cpu, &ioctl, sizeof(unsigned int));

	return 0;
}