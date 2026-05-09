/*
 * v1_handshake.c — TCP 3-way handshake state machine
 *
 * What this stage teaches:
 *   - The SYN/SYN-ACK/ACK sequence and what each packet carries
 *   - Why the ISN must be random (session hijacking prevention)
 *   - Checksum computation: both IP header and TCP pseudo-header
 *   - State machine transitions: CLOSED → SYN_RCVD → ESTABLISHED
 *
 * Build:  make v1
 * Run:    sudo ./v1_handshake
 *         From another terminal: nc 10.0.0.2 8080 -w 2
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <time.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <net/if.h>
#include <linux/if_tun.h>
#include <arpa/inet.h>

#include "tcp.h"

/* Global connection table */
struct tcp_conn conn_table[MAX_CONNS];

/* Our "server" address — traffic to 10.0.0.2:8080 is handled by us */
#define SERVER_IP    0x0a000002U   /* 10.0.0.2 in host byte order */
#define SERVER_PORT  8080

/* ── Checksum (same as v0) ────────────────────────────────────────────────── */

uint16_t checksum(const void *data, int len)
{
    const uint16_t *ptr = (const uint16_t *)data;
    uint32_t sum = 0;
    while (len > 1) { sum += *ptr++; len -= 2; }
    if (len == 1) sum += *(const uint8_t *)ptr;
    while (sum >> 16) sum = (sum & 0xffff) + (sum >> 16);
    return (uint16_t)(~sum);
}

uint16_t tcp_checksum(const struct ip_hdr *ip, const struct tcp_hdr *tcp,
                      const uint8_t *payload, int payload_len)
{
    int tcp_len = TCP_HDR_LEN + payload_len;
    uint8_t *buf = malloc(12 + tcp_len);
    if (!buf) return 0;
    memcpy(buf,     &ip->saddr, 4);
    memcpy(buf + 4, &ip->daddr, 4);
    buf[8] = 0; buf[9] = IPPROTO_TCP;
    uint16_t tl = htons((uint16_t)tcp_len);
    memcpy(buf + 10, &tl, 2);
    memcpy(buf + 12, tcp, TCP_HDR_LEN);
    ((struct tcp_hdr *)(buf + 12))->check = 0;
    if (payload_len > 0) memcpy(buf + 12 + TCP_HDR_LEN, payload, payload_len);
    uint16_t result = checksum(buf, 12 + tcp_len);
    free(buf);
    return result;
}

/* ── TUN device ───────────────────────────────────────────────────────────── */

int open_tun(const char *ifname)
{
    int fd = open("/dev/net/tun", O_RDWR);
    if (fd < 0) { perror("open /dev/net/tun"); return -1; }
    struct ifreq ifr;
    memset(&ifr, 0, sizeof(ifr));
    ifr.ifr_flags = IFF_TUN | IFF_NO_PI;
    strncpy(ifr.ifr_name, ifname, IFNAMSIZ - 1);
    if (ioctl(fd, TUNSETIFF, &ifr) < 0) {
        perror("ioctl TUNSETIFF"); close(fd); return -1;
    }
    fprintf(stderr, "[tun] opened %s (fd=%d)\n", ifname, fd);
    return fd;
}

/* ── Connection table ─────────────────────────────────────────────────────── */

/*
 * find_conn() — linear search through conn_table for a matching 4-tuple.
 *
 * In a production stack this would be a hash table keyed on the 4-tuple.
 * With MAX_CONNS=64, linear search is adequate and simpler.
 */
struct tcp_conn *find_conn(uint32_t src_ip, uint32_t dst_ip,
                           uint16_t src_port, uint16_t dst_port)
{
    for (int i = 0; i < MAX_CONNS; i++) {
        struct tcp_conn *c = &conn_table[i];
        if (!c->used) continue;
        if (c->dst_ip   == src_ip   && c->src_ip   == dst_ip &&
            c->dst_port == src_port && c->src_port == dst_port)
            return c;
    }
    return NULL;
}

struct tcp_conn *new_conn(void)
{
    for (int i = 0; i < MAX_CONNS; i++) {
        if (!conn_table[i].used) {
            memset(&conn_table[i], 0, sizeof(conn_table[i]));
            conn_table[i].used = 1;
            return &conn_table[i];
        }
    }
    fprintf(stderr, "[conn] connection table full (MAX_CONNS=%d)\n", MAX_CONNS);
    return NULL;
}

void free_conn(struct tcp_conn *conn)
{
    if (conn) conn->used = 0;
}

long ms_elapsed(const struct timespec *since)
{
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return (now.tv_sec - since->tv_sec) * 1000L +
           (now.tv_nsec - since->tv_nsec) / 1000000L;
}

/* ── Packet sender ────────────────────────────────────────────────────────── */

/*
 * send_packet() — build a complete IP+TCP frame and write it to the TUN fd.
 *
 * The TUN device expects a raw IP packet (no Ethernet header because we used
 * IFF_TUN, not IFF_TAP). We build:
 *   1. IPv4 header (20 bytes), compute IP header checksum
 *   2. TCP header (20 bytes), compute TCP pseudo-header checksum
 *   3. Payload bytes
 * then write all three contiguously to the fd.
 */
void send_packet(int tun_fd, struct tcp_conn *conn, uint8_t flags,
                 const uint8_t *payload, int payload_len)
{
    int total = IP_HDR_LEN + TCP_HDR_LEN + payload_len;
    uint8_t *pkt = malloc(total);
    if (!pkt) return;
    memset(pkt, 0, total);

    /* Build IP header */
    struct ip_hdr *ip = (struct ip_hdr *)pkt;
    ip->version_ihl = 0x45;                    /* IPv4, IHL=5 (20 bytes) */
    ip->tos         = 0;
    ip->tot_len     = htons((uint16_t)total);
    ip->id          = htons((uint16_t)rand());
    ip->frag_off    = htons(0x4000);            /* DF bit set */
    ip->ttl         = 64;
    ip->protocol    = IPPROTO_TCP;
    ip->saddr       = htonl(conn->src_ip);
    ip->daddr       = htonl(conn->dst_ip);
    ip->check       = checksum(ip, IP_HDR_LEN); /* computed after filling fields */

    /* Build TCP header */
    struct tcp_hdr *tcp = (struct tcp_hdr *)(pkt + IP_HDR_LEN);
    tcp->source   = htons(conn->src_port);
    tcp->dest     = htons(conn->dst_port);
    tcp->seq      = htonl(conn->snd_nxt);
    tcp->ack_seq  = (flags & TCP_FLAG_ACK) ? htonl(conn->rcv_nxt) : 0;
    tcp->doff_res = (TCP_HDR_LEN / 4) << 4;    /* data offset = 5 (no options) */
    tcp->flags    = flags;
    tcp->window   = htons(conn->rcv_wnd ? conn->rcv_wnd : RECV_WINDOW);
    tcp->urg_ptr  = 0;

    /* Copy payload */
    if (payload_len > 0)
        memcpy(pkt + IP_HDR_LEN + TCP_HDR_LEN, payload, payload_len);

    /* TCP checksum (covers pseudo-header + TCP header + payload) */
    tcp->check = tcp_checksum(ip, tcp, payload, payload_len);

    ssize_t written = write(tun_fd, pkt, total);
    if (written < 0) perror("write tun");

    free(pkt);
}

/* ── TCP state machine ────────────────────────────────────────────────────── */

/*
 * handle_syn() — SYN received while LISTENING.
 *
 * Actions:
 *   1. Allocate a connection slot.
 *   2. Generate a random ISN (Initial Sequence Number).
 *      Why random? If ISNs were predictable, an off-path attacker could inject
 *      TCP segments by guessing the sequence numbers (blind TCP injection attack).
 *   3. Set rcv_nxt = remote_seq + 1 (SYN consumes one sequence number).
 *   4. Send SYN-ACK.
 *   5. Transition to SYN_RCVD.
 */
static void handle_syn(int tun_fd, const struct ip_hdr *ip,
                       const struct tcp_hdr *tcp)
{
    struct tcp_conn *conn = new_conn();
    if (!conn) return;

    uint32_t remote_ip   = ntohl(ip->saddr);
    uint16_t remote_port = ntohs(tcp->source);

    conn->state    = TCP_SYN_RCVD;
    conn->src_ip   = SERVER_IP;
    conn->dst_ip   = remote_ip;
    conn->src_port = SERVER_PORT;
    conn->dst_port = remote_port;
    conn->rcv_nxt  = ntohl(tcp->seq) + 1;   /* SYN consumes seq 1 */
    conn->rcv_wnd  = RECV_WINDOW;

    /* Random ISN — in Linux the kernel uses a cryptographic hash of the
     * 4-tuple + a secret key + a time component. We use rand() for simplicity. */
    conn->snd_nxt  = (uint32_t)rand();
    conn->snd_una  = conn->snd_nxt;

    clock_gettime(CLOCK_MONOTONIC, &conn->last_send);

    printf("[v1] SYN from %s:%u — sending SYN-ACK (ISN=%u)\n",
           inet_ntoa(*(struct in_addr *)&ip->saddr), remote_port, conn->snd_nxt);

    send_packet(tun_fd, conn, TCP_FLAG_SYN | TCP_FLAG_ACK, NULL, 0);

    conn->snd_nxt++;  /* SYN-ACK consumes one sequence number */
}

/*
 * handle_ack() — ACK received while in SYN_RCVD.
 *
 * This completes the 3-way handshake. The ACK number should be our ISN+1,
 * confirming the remote received our SYN-ACK.
 */
static void handle_ack(int tun_fd __attribute__((unused)),
                       struct tcp_conn *conn, const struct tcp_hdr *tcp)
{
    if (conn->state != TCP_SYN_RCVD) return;

    uint32_t ack = ntohl(tcp->ack_seq);
    if (ack != conn->snd_nxt) {
        printf("[v1] unexpected ACK number %u (expected %u)\n",
               ack, conn->snd_nxt);
        return;
    }

    conn->snd_una = ack;
    conn->state   = TCP_ESTABLISHED;
    printf("[v1] Connection ESTABLISHED with %u.%u.%u.%u:%u\n",
           (conn->dst_ip >> 24) & 0xff, (conn->dst_ip >> 16) & 0xff,
           (conn->dst_ip >>  8) & 0xff,  conn->dst_ip        & 0xff,
           conn->dst_port);
}

/* ── Main receive loop ────────────────────────────────────────────────────── */

static void run_loop(int tun_fd)
{
    uint8_t buf[65535];

    printf("[v1] listening on 10.0.0.2:%d (try: nc 10.0.0.2 8080 -w 2)\n\n",
           SERVER_PORT);

    for (;;) {
        ssize_t n = read(tun_fd, buf, sizeof(buf));
        if (n < 0) { perror("read"); break; }
        if (n < IP_HDR_LEN) continue;

        struct ip_hdr *ip = (struct ip_hdr *)buf;
        if ((ip->version_ihl >> 4) != 4) continue;
        if (ip->protocol != IPPROTO_TCP) continue;

        int ihl = (ip->version_ihl & 0xf) * 4;
        if (n < ihl + TCP_HDR_LEN) continue;

        struct tcp_hdr *tcp = (struct tcp_hdr *)(buf + ihl);
        uint32_t dst_ip_h   = ntohl(ip->daddr);
        uint16_t dst_port_h = ntohs(tcp->dest);

        /* Only handle packets destined for our server address */
        if (dst_ip_h != SERVER_IP || dst_port_h != SERVER_PORT) continue;

        uint32_t src_ip_h   = ntohl(ip->saddr);
        uint16_t src_port_h = ntohs(tcp->source);

        if (tcp->flags & TCP_FLAG_SYN && !(tcp->flags & TCP_FLAG_ACK)) {
            /* New connection request */
            handle_syn(tun_fd, ip, tcp);
        } else {
            struct tcp_conn *conn = find_conn(src_ip_h, dst_ip_h,
                                              src_port_h, dst_port_h);
            if (!conn) {
                printf("[v1] unknown connection from %u:%u\n",
                       src_ip_h, src_port_h);
                continue;
            }
            if (tcp->flags & TCP_FLAG_ACK)
                handle_ack(tun_fd, conn, tcp);
            if (tcp->flags & TCP_FLAG_FIN) {
                printf("[v1] FIN received — closing connection\n");
                free_conn(conn);
            }
        }
    }
}

/* ── Entry point ──────────────────────────────────────────────────────────── */

int main(void)
{
    srand((unsigned)time(NULL));

    int tun_fd = open_tun("tun0");
    if (tun_fd < 0) {
        fprintf(stderr, "Failed to open TUN. Run as root.\n");
        return 1;
    }

    system("ip addr add 10.0.0.1/24 dev tun0 2>/dev/null || true");
    system("ip link set tun0 up");
    /* Route 10.0.0.2 through tun0 so traffic reaches our process */
    system("ip route add 10.0.0.2/32 dev tun0 2>/dev/null || true");

    run_loop(tun_fd);
    close(tun_fd);
    return 0;
}
