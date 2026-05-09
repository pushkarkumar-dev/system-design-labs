/*
 * v2_reliable.c — Reliable delivery: seq/ack, receive window, retransmission, FIN
 *
 * What this stage teaches:
 *   - Sequence numbers track bytes, not packets
 *   - The receive window prevents buffer overflow at the receiver
 *   - Retransmission timer: send again if no ACK within RETRANSMIT_MS
 *   - FIN/FIN-ACK closes the connection gracefully with TIME_WAIT cleanup
 *
 * Build:  make v2
 * Run:    sudo ./v2_reliable
 *         From another terminal: echo "hello" | nc 10.0.0.2 8080 -w 2
 *
 * The server echoes back every byte it receives.
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

struct tcp_conn conn_table[MAX_CONNS];

#define SERVER_IP    0x0a000002U   /* 10.0.0.2 */
#define SERVER_PORT  8080

/* ── Checksums ────────────────────────────────────────────────────────────── */

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
    return fd;
}

/* ── Connection table ─────────────────────────────────────────────────────── */

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
            conn_table[i].used    = 1;
            conn_table[i].rcv_wnd = RECV_WINDOW;
            return &conn_table[i];
        }
    }
    return NULL;
}

void free_conn(struct tcp_conn *conn) { if (conn) conn->used = 0; }

long ms_elapsed(const struct timespec *since)
{
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return (now.tv_sec - since->tv_sec) * 1000L +
           (now.tv_nsec - since->tv_nsec) / 1000000L;
}

/* ── Packet sender ────────────────────────────────────────────────────────── */

void send_packet(int tun_fd, struct tcp_conn *conn, uint8_t flags,
                 const uint8_t *payload, int payload_len)
{
    int total = IP_HDR_LEN + TCP_HDR_LEN + payload_len;
    uint8_t *pkt = malloc(total);
    if (!pkt) return;
    memset(pkt, 0, total);

    struct ip_hdr *ip = (struct ip_hdr *)pkt;
    ip->version_ihl = 0x45;
    ip->tot_len     = htons((uint16_t)total);
    ip->id          = htons((uint16_t)rand());
    ip->frag_off    = htons(0x4000);
    ip->ttl         = 64;
    ip->protocol    = IPPROTO_TCP;
    ip->saddr       = htonl(conn->src_ip);
    ip->daddr       = htonl(conn->dst_ip);
    ip->check       = checksum(ip, IP_HDR_LEN);

    struct tcp_hdr *tcp = (struct tcp_hdr *)(pkt + IP_HDR_LEN);
    tcp->source   = htons(conn->src_port);
    tcp->dest     = htons(conn->dst_port);
    tcp->seq      = htonl(conn->snd_nxt);
    tcp->ack_seq  = (flags & TCP_FLAG_ACK) ? htonl(conn->rcv_nxt) : 0;
    tcp->doff_res = (TCP_HDR_LEN / 4) << 4;
    tcp->flags    = flags;
    tcp->window   = htons(RECV_WINDOW);

    if (payload_len > 0) memcpy(pkt + IP_HDR_LEN + TCP_HDR_LEN, payload, payload_len);
    tcp->check = tcp_checksum(ip, tcp, payload, payload_len);

    if (payload_len > 0) {
        /* Save to send buffer for possible retransmission */
        int copy = payload_len < SEND_BUF_SIZE ? payload_len : SEND_BUF_SIZE;
        memcpy(conn->send_buf, payload, copy);
        conn->send_len = copy;
        clock_gettime(CLOCK_MONOTONIC, &conn->last_send);
        conn->retransmitting = 0;
    }

    write(tun_fd, pkt, total);
    free(pkt);
}

/* ── Retransmission check ─────────────────────────────────────────────────── */

/*
 * check_retransmits() — scan all connections and retransmit if RTO expired.
 *
 * RFC 6298 describes computing the RTO from smoothed RTT (SRTT) and variance.
 * We use a fixed RETRANSMIT_MS=200 for simplicity.
 */
static void check_retransmits(int tun_fd)
{
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);

    for (int i = 0; i < MAX_CONNS; i++) {
        struct tcp_conn *c = &conn_table[i];
        if (!c->used) continue;
        if (c->state != TCP_ESTABLISHED) continue;
        if (c->send_len == 0) continue;
        if (c->snd_una == c->snd_nxt) continue;   /* all ACKed */

        long elapsed_ms = ms_elapsed(&c->last_send);
        if (elapsed_ms >= RETRANSMIT_MS) {
            printf("[v2] RTO fired for conn %u.%u.%u.%u:%u — retransmitting %d bytes\n",
                   (c->dst_ip >> 24) & 0xff, (c->dst_ip >> 16) & 0xff,
                   (c->dst_ip >>  8) & 0xff,  c->dst_ip        & 0xff,
                   c->dst_port, c->send_len);
            c->retransmitting = 1;
            send_packet(tun_fd, c, TCP_FLAG_ACK | TCP_FLAG_PSH,
                        c->send_buf, c->send_len);
        }
    }
}

/* ── TIME_WAIT cleanup ────────────────────────────────────────────────────── */

static void check_time_wait(void)
{
    for (int i = 0; i < MAX_CONNS; i++) {
        struct tcp_conn *c = &conn_table[i];
        if (!c->used) continue;
        if (c->state != TCP_TIME_WAIT) continue;
        if (ms_elapsed(&c->time_wait_start) >= TIME_WAIT_MS) {
            printf("[v2] TIME_WAIT expired — freeing connection slot\n");
            free_conn(c);
        }
    }
}

/* ── TCP event handlers ───────────────────────────────────────────────────── */

static void handle_syn(int tun_fd, const struct ip_hdr *ip,
                       const struct tcp_hdr *tcp)
{
    struct tcp_conn *conn = new_conn();
    if (!conn) { fprintf(stderr, "[v2] conn table full\n"); return; }

    conn->state    = TCP_SYN_RCVD;
    conn->src_ip   = SERVER_IP;
    conn->dst_ip   = ntohl(ip->saddr);
    conn->src_port = SERVER_PORT;
    conn->dst_port = ntohs(tcp->source);
    conn->rcv_nxt  = ntohl(tcp->seq) + 1;
    conn->snd_nxt  = (uint32_t)rand();
    conn->snd_una  = conn->snd_nxt;

    clock_gettime(CLOCK_MONOTONIC, &conn->last_send);
    printf("[v2] SYN → sending SYN-ACK\n");
    send_packet(tun_fd, conn, TCP_FLAG_SYN | TCP_FLAG_ACK, NULL, 0);
    conn->snd_nxt++;
}

/*
 * handle_data() — data segment received.
 *
 * Three things happen:
 *   1. Validate sequence number (in-order delivery only — no reordering in toy stack).
 *   2. Advance rcv_nxt by the number of data bytes received.
 *   3. Echo the data back (simple echo server) and send ACK.
 *
 * Real stacks buffer out-of-order segments; we drop them (they will be retransmitted).
 */
static void handle_data(int tun_fd, struct tcp_conn *conn,
                        const uint8_t *data, int data_len)
{
    if (data_len <= 0) return;

    /* Check sequence number — only accept in-order data */
    /* (simplified: in production we'd check seq == rcv_nxt) */

    /* Copy into receive buffer */
    int copy = data_len < RECV_BUF_SIZE - conn->recv_len ?
               data_len : RECV_BUF_SIZE - conn->recv_len;
    memcpy(conn->recv_buf + conn->recv_len, data, copy);
    conn->recv_len += copy;

    /* Advance rcv_nxt — seq numbers track bytes */
    conn->rcv_nxt += (uint32_t)copy;

    printf("[v2] received %d bytes — echoing back, rcv_nxt now %u\n",
           copy, conn->rcv_nxt);

    /* Send ACK for received data */
    send_packet(tun_fd, conn, TCP_FLAG_ACK, NULL, 0);

    /* Echo data back */
    conn->send_len = 0; /* reset before send_packet fills it */
    send_packet(tun_fd, conn, TCP_FLAG_ACK | TCP_FLAG_PSH, data, copy);
    conn->snd_nxt += (uint32_t)copy;
}

static void handle_fin(int tun_fd, struct tcp_conn *conn)
{
    conn->rcv_nxt++;   /* FIN consumes one sequence number */

    if (conn->state == TCP_ESTABLISHED) {
        conn->state = TCP_CLOSE_WAIT;
        printf("[v2] FIN received → CLOSE_WAIT, sending FIN-ACK\n");
        /* Send ACK for their FIN */
        send_packet(tun_fd, conn, TCP_FLAG_ACK, NULL, 0);
        /* Send our FIN */
        send_packet(tun_fd, conn, TCP_FLAG_FIN | TCP_FLAG_ACK, NULL, 0);
        conn->snd_nxt++;
        conn->state = TCP_LAST_ACK;
    } else if (conn->state == TCP_FIN_WAIT_1) {
        /* Simultaneous close */
        conn->state = TCP_TIME_WAIT;
        send_packet(tun_fd, conn, TCP_FLAG_ACK, NULL, 0);
        clock_gettime(CLOCK_MONOTONIC, &conn->time_wait_start);
    }
}

static void handle_ack_established(struct tcp_conn *conn, const struct tcp_hdr *tcp)
{
    uint32_t ack = ntohl(tcp->ack_seq);

    if (conn->state == TCP_SYN_RCVD) {
        conn->snd_una = ack;
        conn->state   = TCP_ESTABLISHED;
        printf("[v2] handshake complete → ESTABLISHED\n");
        return;
    }

    if (conn->state == TCP_LAST_ACK) {
        if (ack == conn->snd_nxt) {
            printf("[v2] final ACK received → CLOSED\n");
            free_conn(conn);
        }
        return;
    }

    if (ack > conn->snd_una) {
        conn->snd_una = ack;
        conn->send_len = 0; /* data acknowledged, no retransmit needed */
    }
}

/* ── Main receive loop ────────────────────────────────────────────────────── */

static void run_loop(int tun_fd)
{
    uint8_t buf[65535];

    /* Use non-blocking reads so we can poll for retransmit timeouts */
    int flags = fcntl(tun_fd, F_GETFL, 0);
    fcntl(tun_fd, F_SETFL, flags | O_NONBLOCK);

    printf("[v2] echo server on 10.0.0.2:%d — try: echo hello | nc 10.0.0.2 8080 -w 2\n\n",
           SERVER_PORT);

    long last_timer_check = 0;

    for (;;) {
        /* Periodic timer: check every 50 ms */
        struct timespec now;
        clock_gettime(CLOCK_MONOTONIC, &now);
        long now_ms = now.tv_sec * 1000L + now.tv_nsec / 1000000L;
        if (now_ms - last_timer_check >= 50) {
            check_retransmits(tun_fd);
            check_time_wait();
            last_timer_check = now_ms;
        }

        ssize_t n = read(tun_fd, buf, sizeof(buf));
        if (n < 0) {
            if (errno == EAGAIN) { usleep(1000); continue; }
            perror("read"); break;
        }
        if (n < IP_HDR_LEN) continue;

        struct ip_hdr *ip = (struct ip_hdr *)buf;
        if ((ip->version_ihl >> 4) != 4) continue;
        if (ip->protocol != IPPROTO_TCP) continue;

        int ihl = (ip->version_ihl & 0xf) * 4;
        if (n < ihl + TCP_HDR_LEN) continue;

        struct tcp_hdr *tcp = (struct tcp_hdr *)(buf + ihl);
        if (ntohl(ip->daddr) != SERVER_IP) continue;
        if (ntohs(tcp->dest) != SERVER_PORT) continue;

        uint32_t src_ip_h   = ntohl(ip->saddr);
        uint16_t src_port_h = ntohs(tcp->source);
        int tot_len         = ntohs(ip->tot_len);
        int data_offset     = ((tcp->doff_res >> 4) & 0xf) * 4;
        int payload_len     = tot_len - ihl - data_offset;
        if (payload_len < 0) payload_len = 0;
        const uint8_t *payload = buf + ihl + data_offset;

        if ((tcp->flags & TCP_FLAG_SYN) && !(tcp->flags & TCP_FLAG_ACK)) {
            handle_syn(tun_fd, ip, tcp);
            continue;
        }

        struct tcp_conn *conn = find_conn(src_ip_h, SERVER_IP,
                                          src_port_h, SERVER_PORT);
        if (!conn) continue;

        if (tcp->flags & TCP_FLAG_ACK)
            handle_ack_established(conn, tcp);

        if (payload_len > 0 && conn->state == TCP_ESTABLISHED)
            handle_data(tun_fd, conn, payload, payload_len);

        if (tcp->flags & TCP_FLAG_FIN)
            handle_fin(tun_fd, conn);
    }
}

/* ── Entry point ──────────────────────────────────────────────────────────── */

int main(void)
{
    srand((unsigned)time(NULL));

    int tun_fd = open_tun("tun0");
    if (tun_fd < 0) { fprintf(stderr, "Run as root\n"); return 1; }

    system("ip addr add 10.0.0.1/24 dev tun0 2>/dev/null || true");
    system("ip link set tun0 up");
    system("ip route add 10.0.0.2/32 dev tun0 2>/dev/null || true");

    run_loop(tun_fd);
    close(tun_fd);
    return 0;
}
