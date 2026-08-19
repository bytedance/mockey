//go:build darwin && arm64

package common

/*
#include <mach/mach.h>
#include <mach/mach_vm.h>
#include <stdint.h>

// Walk the VM map to find a free hole of `size` within [lo, hi], then allocate
// it with VM_FLAGS_FIXED. Searches upward from `target` first, then downward.
// Returns 0 on success and writes the address to *out.
int alloc_in_range(uint64_t target, uint64_t lo, uint64_t hi, uint64_t size, uint64_t *out) {
    // --- upward: walk regions from target, allocate in the first hole ---
    mach_vm_address_t addr = (mach_vm_address_t)target;
    for (int i = 0; i < 4096; i++) {
        mach_vm_address_t a = addr;
        mach_vm_size_t s = 0;
        vm_region_basic_info_data_64_t info;
        mach_msg_type_number_t cnt = VM_REGION_BASIC_INFO_COUNT_64;
        mach_port_t obj;
        kern_return_t kr = mach_vm_region(mach_task_self(), &a, &s, VM_REGION_BASIC_INFO_64,
                                          (vm_region_info_t)&info, &cnt, &obj);
        uint64_t hole_start, hole_end;
        if (kr != KERN_SUCCESS) {
            // nothing mapped above: the whole remaining range is a hole
            hole_start = addr;
            hole_end = hi;
        } else if ((uint64_t)a > (uint64_t)addr) {
            hole_start = addr;
            hole_end = (uint64_t)a;
        } else {
            addr = (mach_vm_address_t)((uint64_t)a + (uint64_t)s);
            if ((uint64_t)addr > hi) break;
            continue;
        }
        if (hole_end > hi) hole_end = hi;
        if (hole_start >= hole_end) break;
        if (hole_end - hole_start >= size) {
            mach_vm_address_t want = (mach_vm_address_t)hole_start;
            kern_return_t k2 = mach_vm_allocate(mach_task_self(), &want, size, VM_FLAGS_FIXED);
            if (k2 == KERN_SUCCESS) { *out = (uint64_t)want; return 0; }
        }
        addr = (mach_vm_address_t)hole_end;
        if ((uint64_t)addr > hi) break;
    }

    // --- downward: probe page-aligned candidates below target ---
    for (uint64_t cand = target > size ? ((target - size) & ~(uint64_t)(size - 1)) : 0;
         cand >= lo && cand > 0; cand -= size) {
        mach_vm_address_t a = (mach_vm_address_t)cand;
        mach_vm_size_t s = 0;
        vm_region_basic_info_data_64_t info;
        mach_msg_type_number_t cnt = VM_REGION_BASIC_INFO_COUNT_64;
        mach_port_t obj;
        kern_return_t kr = mach_vm_region(mach_task_self(), &a, &s, VM_REGION_BASIC_INFO_64,
                                          (vm_region_info_t)&info, &cnt, &obj);
        // free if no region covers cand
        if (kr == KERN_SUCCESS && (uint64_t)a <= cand && cand < (uint64_t)a + (uint64_t)s) continue;
        mach_vm_address_t want = (mach_vm_address_t)cand;
        if (mach_vm_allocate(mach_task_self(), &want, size, VM_FLAGS_FIXED) == KERN_SUCCESS) {
            *out = (uint64_t)want;
            return 0;
        }
    }
    return -1;
}
*/
import "C"

// allocatePageWithin allocates one page whose address is within `reach` bytes
// of target, so a short PC-relative branch from target can reach it.
// Returns nil when no free page exists in that window.
//
// This is deliberately distinct from the exported AllocatePageNear in page.go,
// which serves the amd64 E9 path: that one takes no reach because rel32 fixes
// the window at +/-2GB and it reports an error, whereas this one is the
// darwin/arm64 mach allocator for the +/-128MB B range and reports nil. They
// used to share a name, which does not compile on darwin/arm64 where both are
// built. Unexported because the only caller is the trampoline pool next door.
func allocatePageWithin(target uintptr, reach uintptr) []byte {
	lo := uintptr(0)
	if target > reach {
		lo = target - reach
	}
	hi := target + reach
	var out C.uint64_t
	if rc := C.alloc_in_range(C.uint64_t(target), C.uint64_t(lo), C.uint64_t(hi),
		C.uint64_t(pageSize), &out); rc != 0 {
		return nil
	}
	addr := uintptr(out)
	// paranoia: never hand back something out of reach
	if addr < lo || addr >= hi {
		return nil
	}
	return BytesOf(addr, int(pageSize))
}
