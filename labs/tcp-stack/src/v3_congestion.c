/*
 * v3_congestion.c — Slow start + congestion avoidance + fast retransmit (TCP Reno)
 *
 * What this stage teaches:
 *   - Slow start: cwnd doubles per RTT until ssthresh — exponential growth
 *   - Congestion avoidance: additive increase (cwnd += MSS^2/cwnd per ACK)
 *   - Fast retransmit: 3 duplicate ACKs signal packet loss without waiting for timeout
 *   - Multiplicative decrease: on loss, ssthresh = cwnd/2, cwnd = ssthresh
 *
 * Build:  make v3
 * Run:    sudo ./v3_congestion
 *         Throughput printed every second to stderr.
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

#define SERVER_IP    0x0a000002U
#define SERVER_PORT  8080

/* ── Throughput tracking ─────────────────────────────────────────────────── */

static uint64_t bytes_sent_total   = 0;
static uint64_t bytes_sent_window  = 0;
static struct timespec window_start;

static void throughput_init(void)
{
    clock_gettime(CLOCK_MONOTONIC, &window_start);
}

static void throughput_tick(int bytes)
{
    bytes_sent_total  += (uint64_t)bytes;
    bytes_sent_window += (uint64_t)bytes;

    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    long elapsed_ms = (now.tv_sec - window_start.tv_sec) * 1000L +
                      (now.tv_nsec - window_start.tv_nsec) / 1000000L;
    if (elapsed_ms >= 1000) {
        double mb_per_sec = (double)bytes_sent_window / (double)elapsed_ms / 1000.0;
        fprintf(stderr, "[v3] throughput: %.1f MB/s  (total %.2f MB sent)\n",
                mb_per_sec,
                (double)bytes_sent_total / 1048576.0);
        bytes_sent_window = 0;
        clock_gettime(CLOCK_MONOTONIC, &window_start);
    }
}

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
            conn_table[i].used      = 1;
            conn_table[i].rcv_wnd  = RECV_WINDOW;
            /* Congestion control initial state (RFC 5681) */
            conn_table[i].cwnd     = MSS;          /* start at 1 MSS */
            conn_table[i].ssthresh = 65535;         /* initial ssthresh */
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
        int copy = payload_len < SEND_BUF_SIZE ? payload_len : SEND_BUF_SIZE;
        memcpy(conn->send_buf, payload, copy);
        conn->send_len = copy;
        clock_gettime(CLOCK_MONOTONIC, &conn->last_send);
        throughput_tick(copy);
    }

    write(tun_fd, pkt, total);
    free(pkt);
}

/* ── Congestion control ───────────────────────────────────────────────────── */

/*
 * on_ack() — update congestion window on each new ACK (not a duplicate).
 *
 * Slow start (cwnd < ssthresh):
 *   cwnd += MSS per ACK → effectively doubles per RTT (exponential growth)
 *
 * Congestion avoidance (cwnd >= ssthresh):
 *   cwnd += MSS * MSS / cwnd per ACK → grows by roughly 1 MSS per RTT (linear)
 */
static void on_ack(struct tcp_conn *conn)
{
    if (conn->cwnd < conn->ssthresh) {
        /* Slow start: increase by one MSS per ACK */
        conn->cwnd += MSS;
        fprintf(stderr, "[cc] slow-start  cwnd=%u ssthresh=%u\n",
                conn->cwnd, conn->ssthresh);
    } else {
        /* Congestion avoidance: AIMD linear increase */
        conn->cwnd += (MSS * MSS) / conn->cwnd;
        fprintf(stderr, "[cc] cong-avoid  cwnd=%u ssthresh=%u\n",
                conn->cwnd, conn->ssthresh);
    }
    conn->dup_ack_count = 0;
}

/*
 * on_duplicate_ack() — 3 duplicate ACKs signal a lost segment (RFC 5681 §3.2).
 *
 * TCP Reno fast retransmit / fast recovery:
 *   1. ssthresh = max(cwnd/2, 2*MSS)   — multiplicative decrease
 *   2. cwnd     = ssthresh              — enter CA immediately (not slow start)
 *   3. Retransmit the missing segment
 *
 * The key insight: 3 duplicate ACKs mean the receiver is still receiving
 * (out-of-order) segments, so the path is not fully congested — we can recover
 * faster than a full timeout would require.
 */
static void on_duplicate_ack(int tun_fd, struct tcp_conn *conn)
{
    conn->dup_ack_count++;
    if (conn->dup_ack_count < 3) return;

    /* Fast retransmit triggered */
    conn->ssthresh = conn->cwnd / 2;
    if (conn->ssthresh < 2 * MSS) conn->ssthresh = 2 * MSS;
    conn->cwnd = conn->ssthresh;
    conn->dup_ack_count = 0;

    fprintf(stderr, "[cc] fast-retransmit  new ssthresh=%u cwnd=%u\n",
            conn->ssthresh, conn->cwnd);

    if (conn->send_len > 0) {
        send_packet(tun_fd, conn, TCP_FLAG_ACK | TCP_FLAG_PSH,
                    conn->send_buf, conn->send_len);
    }
}

/*
 * on_timeout() — RTO expired, must assume severe congestion.
 *
 * Unlike fast retransmit (which uses ssthresh as new cwnd), timeout goes back
 * all the way to slow start: cwnd = 1 MSS.
 */
static void on_timeout(int tun_fd, struct tcp_conn *conn)
{
    fprintf(stderr, "[cc] RTO timeout  old cwnd=%u → cwnd=MSS=%d ssthresh=%u\n",
            conn->cwnd, MSS, conn->cwnd / 2);

    conn->ssthresh = conn->cwnd / 2;
    if (conn->ssthresh < 2 * MSS) conn->ssthresh = 2 * MSS;
    conn->cwnd = MSS;            /* back to slow start */
    conn->dup_ack_count = 0;

    if (conn->send_len > 0) {
        send_packet(tun_fd, conn, TCP_FLAG_ACK | TCP_FLAG_PSH,
                    conn->send_buf, conn->send_len);
    }
}

/* ── Retransmit and TIME_WAIT timers ─────────────────────────────────────── */

static void check_retransmits(int tun_fd)
{
    for (int i = 0; i < MAX_CONNS; i++) {
        struct tcp_conn *c = &conn_table[i];
        if (!c->used || c->state != TCP_ESTABLISHED) continue;
        if (c->send_len == 0 || c->snd_una == c->snd_nxt) continue;
        if (ms_elapsed(&c->last_send) >= RETRANSMIT_MS)
            on_timeout(tun_fd, c);
    }
}

static void check_time_wait(void)
{
    for (int i = 0; i < MAX_CONNS; i++) {
        struct tcp_conn *c = &conn_table[i];
        if (!c->used || c->state != TCP_TIME_WAIT) continue;
        if (ms_elapsed(&c->time_wait_start) >= TIME_WAIT_MS)
            free_conn(c);
    }
}

/* ── TCP event handlers ───────────────────────────────────────────────────── */

static void handle_syn(int tun_fd, const struct ip_hdr *ip,
                       const struct tcp_hdr *tcp)
{
    struct tcp_conn *conn = new_conn();
    if (!conn) return;
    conn->state    = TCP_SYN_RCVD;
    conn->src_ip   = SERVER_IP;
    conn->dst_ip   = ntohl(ip->saddr);
    conn->src_port = SERVER_PORT;
    conn->dst_port = ntohs(tcp->source);
    conn->rcv_nxt  = ntohl(tcp->seq) + 1;
    conn->snd_nxt  = (uint32_t)rand();
    conn->snd_una  = conn->snd_nxt;
    clock_gettime(CLOCK_MONOTONIC, &conn->last_send);
    send_packet(tun_fd, conn, TCP_FLAG_SYN | TCP_FLAG_ACK, NULL, 0);
    conn->snd_nxt++;
}

static void handle_ack(int tun_fd, struct tcp_conn *conn,
                       const struct tcp_hdr *tcp)
{
    uint32_t ack = ntohl(tcp->ack_seq);

    if (conn->state == TCP_SYN_RCVD) {
        conn->snd_una = ack;
        conn->state   = TCP_ESTABLISHED;
        printf("[v3] ESTABLISHED cwnd=%u ssthresh=%u\n",
               conn->cwnd, conn->ssthresh);
        return;
    }
    if (conn->state == TCP_LAST_ACK) {
        if (ack == conn->snd_nxt) free_conn(conn);
        return;
    }

    if (ack > conn->snd_una) {
        /* New ACK — advance snd_una and update cwnd */
        conn->snd_una = ack;
        conn->send_len = 0;
        on_ack(conn);
    } else if (ack == conn->snd_una && conn->send_len > 0) {
        /* Duplicate ACK */
        on_duplicate_ack(tun_fd, conn);
    }

    (void)tun_fd;
}

static void handle_data(int tun_fd, struct tcp_conn *conn,
                        const uint8_t *data, int data_len)
{
    if (data_len <= 0) return;
    int copy = data_len < RECV_BUF_SIZE ? data_len : RECV_BUF_SIZE;
    conn->rcv_nxt += (uint32_t)copy;
    /* Send ACK */
    send_packet(tun_fd, conn, TCP_FLAG_ACK, NULL, 0);
    /* Echo data back (capped by cwnd) */
    int send_len = copy < (int)conn->cwnd ? copy : (int)conn->cwnd;
    send_packet(tun_fd, conn, TCP_FLAG_ACK | TCP_FLAG_PSH, data, send_len);
    conn->snd_nxt += (uint32_t)send_len;
}

static void handle_fin(int tun_fd, struct tcp_conn *conn)
{
    conn->rcv_nxt++;
    if (conn->state == TCP_ESTABLISHED) {
        conn->state = TCP_CLOSE_WAIT;
        send_packet(tun_fd, conn, TCP_FLAG_ACK, NULL, 0);
        send_packet(tun_fd, conn, TCP_FLAG_FIN | TCP_FLAG_ACK, NULL, 0);
        conn->snd_nxt++;
        conn->state = TCP_LAST_ACK;
    }
}

/* ── Main receive loop ────────────────────────────────────────────────────── */

static void run_loop(int tun_fd)
{
    uint8_t buf[65535];
    int flags = fcntl(tun_fd, F_GETFL, 0);
    fcntl(tun_fd, F_SETFL, flags | O_NONBLOCK);

    throughput_init();
    printf("[v3] congestion-controlled echo server on 10.0.0.2:%d\n", SERVER_PORT);
    printf("[v3] cwnd starts at MSS=%d, ssthresh=65535\n\n", MSS);

    long last_timer = 0;

    for (;;) {
        struct timespec now;
        clock_gettime(CLOCK_MONOTONIC, &now);
        long now_ms = now.tv_sec * 1000L + now.tv_nsec / 1000000L;
        if (now_ms - last_timer >= 50) {
            check_retransmits(tun_fd);
            check_time_wait();
            last_timer = now_ms;
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
        int tot_len     = ntohs(ip->tot_len);
        int data_offset = ((tcp->doff_res >> 4) & 0xf) * 4;
        int payload_len = tot_len - ihl - data_offset;
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
            handle_ack(tun_fd, conn, tcp);

        if (payload_len > 0 && conn->used && conn->state == TCP_ESTABLISHED)
            handle_data(tun_fd, conn, payload, payload_len);

        if (tcp->flags & TCP_FLAG_FIN && conn->used)
            handle_fin(tun_fd, conn);
    }
}

/* ── Entry point ──────────────────────────────────────────────────────────── */

int main(void)
{
    srand((unsigned)time(NULL));
    throughput_init();

    int tun_fd = open_tun("tun0");
    if (tun_fd < 0) { fprintf(stderr, "Run as root\n"); return 1; }

    system("ip addr add 10.0.0.1/24 dev tun0 2>/dev/null || true");
    system("ip link set tun0 up");
    system("ip route add 10.0.0.2/32 dev tun0 2>/dev/null || true");

    run_loop(tun_fd);
    close(tun_fd);
    return 0;
}
