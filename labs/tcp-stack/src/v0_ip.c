/*
 * v0_ip.c — TUN device setup and IPv4/TCP header parsing
 *
 * What this stage teaches:
 *   - A TUN device is a virtual NIC exposed as a file descriptor
 *   - IPv4 headers are 20 bytes minimum, all fields are big-endian
 *   - TCP headers are 20 bytes minimum; flags are individual bits
 *
 * Build:   make v0
 * Run:     sudo ./v0_ip
 *          (sets up tun0 at 10.0.0.1/24 automatically)
 *
 * From another terminal, send some traffic:
 *   ping -I tun0 10.0.0.2   (if you configure the route)
 *   OR just watch with tcpdump -i tun0
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <net/if.h>
#include <linux/if_tun.h>
#include <arpa/inet.h>

#include "tcp.h"

/* Global conn table (unused in v0, defined here so tcp.h compiles) */
struct tcp_conn conn_table[MAX_CONNS];

/* ── Checksum implementations ─────────────────────────────────────────────── */

/*
 * checksum() — standard 16-bit ones-complement sum used by both IP and TCP.
 *
 * The algorithm:
 *   1. Treat the data as an array of 16-bit words.
 *   2. Sum all words. If there's an odd byte at the end, zero-pad to 16 bits.
 *   3. Fold any carry bits back into the low 16 bits (sum may be 32-bit).
 *   4. Return the bitwise NOT (ones complement).
 */
uint16_t checksum(const void *data, int len)
{
    const uint16_t *ptr = (const uint16_t *)data;
    uint32_t sum = 0;

    while (len > 1) {
        sum += *ptr++;
        len -= 2;
    }
    /* Odd byte: pad with zero */
    if (len == 1)
        sum += *(const uint8_t *)ptr;

    /* Fold 32-bit sum into 16 bits */
    while (sum >> 16)
        sum = (sum & 0xffff) + (sum >> 16);

    return (uint16_t)(~sum);
}

/*
 * tcp_checksum() — TCP uses a "pseudo-header" in its checksum calculation.
 *
 * The pseudo-header (12 bytes) prefixes the TCP header + payload and covers:
 *   - source IP (4 bytes)
 *   - destination IP (4 bytes)
 *   - zero byte (1 byte)
 *   - protocol (1 byte, = 6 for TCP)
 *   - TCP segment length (2 bytes) = TCP header + payload length
 *
 * This ties the TCP checksum to the IP addresses, preventing a packet
 * delivered to the wrong host from silently passing the TCP checksum.
 */
uint16_t tcp_checksum(const struct ip_hdr *ip, const struct tcp_hdr *tcp,
                      const uint8_t *payload, int payload_len)
{
    int tcp_len = TCP_HDR_LEN + payload_len;

    /* Build pseudo-header + TCP header + payload in a temporary buffer */
    uint8_t *buf = malloc(12 + tcp_len);
    if (!buf) return 0;

    /* Pseudo-header */
    memcpy(buf,     &ip->saddr, 4);
    memcpy(buf + 4, &ip->daddr, 4);
    buf[8] = 0;
    buf[9] = IPPROTO_TCP;
    uint16_t tcp_len_be = htons((uint16_t)tcp_len);
    memcpy(buf + 10, &tcp_len_be, 2);

    /* TCP header (with checksum field zeroed) */
    memcpy(buf + 12, tcp, TCP_HDR_LEN);
    ((struct tcp_hdr *)(buf + 12))->check = 0;

    /* Payload */
    if (payload_len > 0)
        memcpy(buf + 12 + TCP_HDR_LEN, payload, payload_len);

    uint16_t result = checksum(buf, 12 + tcp_len);
    free(buf);
    return result;
}

/* ── TUN device ───────────────────────────────────────────────────────────── */

/*
 * open_tun() — open /dev/net/tun and configure it as a TUN (layer 3) device.
 *
 * IFF_TUN:    layer-3 device — we receive raw IP packets (no Ethernet header)
 * IFF_NO_PI:  no extra 4-byte "packet info" prefix per frame (simpler parsing)
 *
 * After this call the kernel creates a virtual NIC named `ifname`.
 * Reading from the fd returns one IP packet per read() call.
 * Writing to the fd injects one IP packet into the kernel's network stack.
 */
int open_tun(const char *ifname)
{
    int fd = open("/dev/net/tun", O_RDWR);
    if (fd < 0) {
        perror("open /dev/net/tun");
        return -1;
    }

    struct ifreq ifr;
    memset(&ifr, 0, sizeof(ifr));
    ifr.ifr_flags = IFF_TUN | IFF_NO_PI;
    strncpy(ifr.ifr_name, ifname, IFNAMSIZ - 1);

    if (ioctl(fd, TUNSETIFF, &ifr) < 0) {
        perror("ioctl TUNSETIFF");
        close(fd);
        return -1;
    }

    fprintf(stderr, "[tun] opened %s (fd=%d)\n", ifname, fd);
    return fd;
}

/* ── Hexdump helper ───────────────────────────────────────────────────────── */

static void hexdump(const uint8_t *data, int len)
{
    for (int i = 0; i < len; i++) {
        if (i % 16 == 0) printf("  %04x  ", i);
        printf("%02x ", data[i]);
        if (i % 16 == 15) printf("\n");
    }
    if (len % 16 != 0) printf("\n");
}

/* ── Flag decoder ─────────────────────────────────────────────────────────── */

static void print_tcp_flags(uint8_t flags)
{
    printf("[");
    if (flags & TCP_FLAG_SYN) printf("SYN ");
    if (flags & TCP_FLAG_ACK) printf("ACK ");
    if (flags & TCP_FLAG_FIN) printf("FIN ");
    if (flags & TCP_FLAG_RST) printf("RST ");
    if (flags & TCP_FLAG_PSH) printf("PSH ");
    if (flags & TCP_FLAG_URG) printf("URG ");
    printf("]");
}

/* ── Main receive loop ────────────────────────────────────────────────────── */

/*
 * run_loop() — read IP packets from the TUN fd and parse their headers.
 *
 * Each read() returns exactly one IP packet. The packet starts with the
 * 20-byte IPv4 header (IHL=5 means no options, which is the common case).
 */
static void run_loop(int tun_fd)
{
    uint8_t buf[65535];

    printf("[v0] listening on tun0 — send traffic to 10.0.0.2 to see packets\n");
    printf("[v0] (run: ping -c1 10.0.0.2 in another terminal)\n\n");

    for (;;) {
        ssize_t n = read(tun_fd, buf, sizeof(buf));
        if (n < 0) {
            perror("read tun");
            break;
        }
        if (n < IP_HDR_LEN) {
            printf("[v0] short packet (%zd bytes), skipping\n", n);
            continue;
        }

        struct ip_hdr *ip = (struct ip_hdr *)buf;

        /* Only handle IPv4 */
        int version = (ip->version_ihl >> 4) & 0xf;
        if (version != 4) {
            printf("[v0] non-IPv4 (version=%d), skipping\n", version);
            continue;
        }

        int ihl = (ip->version_ihl & 0xf) * 4;  /* header length in bytes */
        int tot_len = ntohs(ip->tot_len);

        /* Decode source/dest IPs */
        char src_str[INET_ADDRSTRLEN], dst_str[INET_ADDRSTRLEN];
        struct in_addr src_addr = { ip->saddr };
        struct in_addr dst_addr = { ip->daddr };
        inet_ntop(AF_INET, &src_addr, src_str, sizeof(src_str));
        inet_ntop(AF_INET, &dst_addr, dst_str, sizeof(dst_str));

        printf("[v0] IP  src=%-16s dst=%-16s proto=%d total=%d bytes\n",
               src_str, dst_str, ip->protocol, tot_len);

        /* Parse TCP header if present */
        if (ip->protocol == IPPROTO_TCP && n >= ihl + TCP_HDR_LEN) {
            struct tcp_hdr *tcp = (struct tcp_hdr *)(buf + ihl);
            int data_offset = ((tcp->doff_res >> 4) & 0xf) * 4;
            int payload_len = tot_len - ihl - data_offset;
            if (payload_len < 0) payload_len = 0;

            printf("[v0] TCP src_port=%-5u dst_port=%-5u seq=%-10u ack=%-10u flags=",
                   ntohs(tcp->source), ntohs(tcp->dest),
                   ntohl(tcp->seq), ntohl(tcp->ack_seq));
            print_tcp_flags(tcp->flags);
            printf(" window=%u payload=%d bytes\n",
                   ntohs(tcp->window), payload_len);

            /* Hexdump first 16 bytes of TCP payload */
            if (payload_len > 0) {
                int dump_len = payload_len < 16 ? payload_len : 16;
                printf("[v0] payload hex dump (first %d bytes):\n", dump_len);
                hexdump(buf + ihl + data_offset, dump_len);
            }
        }

        printf("\n");
    }
}

/* ── Entry point ──────────────────────────────────────────────────────────── */

int main(void)
{
    /* Must be root to open /dev/net/tun and configure the interface */
    int tun_fd = open_tun("tun0");
    if (tun_fd < 0) {
        fprintf(stderr, "Failed to open TUN device. Run as root.\n");
        return 1;
    }

    /*
     * Configure the tun0 interface.
     *
     * We assign 10.0.0.1 to tun0 (the "host" side).
     * Traffic sent to 10.0.0.2 will be routed through tun0 and appear
     * as packets on our fd.
     *
     * These commands are equivalent to running:
     *   sudo ip addr add 10.0.0.1/24 dev tun0
     *   sudo ip link set tun0 up
     */
    system("ip addr add 10.0.0.1/24 dev tun0 2>/dev/null || true");
    system("ip link set tun0 up");

    run_loop(tun_fd);

    close(tun_fd);
    return 0;
}
