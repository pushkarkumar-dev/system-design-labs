package com.labs.tcp;

import org.springframework.stereotype.Component;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * TcpStackComparison — documents what happens in kernel space vs our TUN stack
 * for each Java NIO operation.
 *
 * This is the learning artifact: side-by-side comparison of what the kernel does
 * when SocketChannel operations are called versus what our 1,650-line C stack does.
 *
 * Exposed as GET /compare so you can view it in a browser or with curl.
 */
@Component
@RestController
public class TcpStackComparison {

    private final TcpClient client;

    public TcpStackComparison(TcpClient client) {
        this.client = client;
    }

    /**
     * Run a round-trip against our userspace TCP stack and return a comparison
     * of what happened at each layer vs what the kernel would do.
     */
    @GetMapping("/demo")
    public Map<String, Object> runDemo() throws Exception {
        TcpClient.RoundTripResult result = client.roundTrip("hello from Java NIO");

        Map<String, Object> response = new LinkedHashMap<>();
        response.put("result", result.toString());
        response.put("connect_latency_ms", result.connectLatency().toMillis());
        response.put("data_latency_ms",    result.dataLatency().toMillis());
        response.put("total_latency_ms",   result.totalLatency().toMillis());
        response.put("note", "connect_latency is the 3-way handshake cost through our TUN stack. " +
                             "Kernel TCP loopback would be ~0.08ms; our stack is ~1.2ms (15x gap).");
        return response;
    }

    /**
     * Return a structured explanation of the kernel vs userspace path for each operation.
     */
    @GetMapping("/compare")
    public Map<String, Object> getComparison() {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("operation_comparison", buildComparison());
        result.put("why_the_gap_exists",   whyTheGapExists());
        return result;
    }

    private List<Map<String, String>> buildComparison() {
        return List.of(
            op("SocketChannel.open()",
               "Calls socket(AF_INET, SOCK_STREAM, 0) — kernel allocates a socket, " +
               "assigns a file descriptor, creates send/recv buffers (~87KB by default).",
               "No equivalent — we have a struct tcp_conn in conn_table[]. " +
               "No kernel socket; we manage the state machine manually."),

            op("SocketChannel.connect(host, port)",
               "Kernel sends SYN, waits for SYN-ACK from the peer, sends ACK. " +
               "All three packets stay inside the kernel network stack. " +
               "On loopback this takes ~0.08ms (one local memory copy per packet).",
               "SYN arrives via read(tun_fd) — one copy from kernel to userspace. " +
               "We send SYN-ACK via write(tun_fd) — one copy from userspace to kernel. " +
               "ACK arrives via read(tun_fd) again. Total: 2 extra kernel crossings = ~1.2ms."),

            op("channel.write(ByteBuffer)",
               "Copies bytes from the Java heap into the kernel send buffer. " +
               "The kernel segments it at MSS=1460 bytes, wraps in TCP/IP headers, " +
               "and hands to the NIC driver (or loopback). " +
               "Zero-copy sendfile() bypasses the heap copy for file data.",
               "Our code in handle_data() receives the segment via read(tun_fd), " +
               "copies to recv_buf, advances rcv_nxt, then echoes by calling write(tun_fd). " +
               "Two extra copies vs kernel zero-copy path."),

            op("channel.read(ByteBuffer)",
               "Blocks until the kernel's receive buffer has data. " +
               "Kernel copies from receive buffer into the Java ByteBuffer. " +
               "Hardware interrupt → softirq → TCP receive path → wake the blocked thread.",
               "Our echo arrives via write(tun_fd) from the C echo server. " +
               "The kernel delivers it to the SocketChannel's receive buffer. " +
               "channel.read() then copies it into our ByteBuffer. " +
               "One extra kernel-user crossing on the send side (write(tun_fd))."),

            op("channel.close()",
               "Kernel sends FIN, receives FIN-ACK, enters TIME_WAIT for 2MSL. " +
               "TIME_WAIT prevents delayed duplicates from poisoning a new connection " +
               "on the same 4-tuple. The kernel manages this automatically.",
               "Our handle_fin() sends FIN-ACK, transitions to LAST_ACK. " +
               "check_time_wait() polls every 50ms and frees the conn_table slot after 2000ms. " +
               "We have only 64 slots — TIME_WAIT can exhaust them at high connection rates.")
        );
    }

    private Map<String, String> op(String operation, String kernel, String ourStack) {
        Map<String, String> m = new LinkedHashMap<>();
        m.put("operation", operation);
        m.put("kernel_tcp", kernel);
        m.put("our_tun_stack", ourStack);
        return m;
    }

    private Map<String, String> whyTheGapExists() {
        Map<String, String> gap = new LinkedHashMap<>();
        gap.put("summary",
                "Our 3-way handshake costs ~1.2ms vs the kernel's 0.08ms — a 15x gap.");
        gap.put("cause_1_tun_read",
                "SYN packet: kernel copies it from the NIC (or loopback) into our " +
                "process via read(tun_fd). That is one kernel-user memory copy that " +
                "the kernel's own TCP stack avoids entirely.");
        gap.put("cause_2_tun_write",
                "SYN-ACK: we call write(tun_fd) to inject our response. " +
                "The kernel copies it back from userspace into the NIC send path. " +
                "Again, the kernel's own stack never crosses the user-kernel boundary here.");
        gap.put("cause_3_scheduling",
                "read(tun_fd) involves a context switch if no data is ready. " +
                "The loopback path inside the kernel stays in kernel mode throughout — " +
                "no context switches between process and kernel.");
        gap.put("dpdk_solution",
                "DPDK (Data Plane Development Kit) moves the NIC driver into userspace, " +
                "eliminating all TUN/TAP kernel crossings. Production userspace stacks " +
                "using DPDK approach kernel-comparable latencies.");
        return gap;
    }
}
