/*
 * tcp.h — shared structs and constants for the userspace TCP stack
 *
 * All wire-format fields are in network byte order (big-endian).
 * Use htonl/htons/ntohl/ntohs before writing to / after reading from headers.
 */

#ifndef TCP_H
#define TCP_H

#include <stdint.h>
#include <arpa/inet.h>  /* htonl, htons, ntohl, ntohs */
#include <time.h>

/* ── Configuration ─────────────────────────────────────────────────────────── */
#define MAX_CONNS       64
#define MSS             1460
#define RETRANSMIT_MS   200
#define TIME_WAIT_MS    2000
#define RECV_WINDOW     4096
#define SEND_BUF_SIZE   4096
#define RECV_BUF_SIZE   4096

/* ── IPv4 header (20 bytes, no options) ────────────────────────────────────── */
struct ip_hdr {
    uint8_t  version_ihl;   /* version (4 bits) | IHL in 32-bit words (4 bits) */
    uint8_t  tos;           /* type of service / DSCP */
    uint16_t tot_len;       /* total length including header (network byte order) */
    uint16_t id;            /* identification */
    uint16_t frag_off;      /* flags (3 bits) | fragment offset (13 bits) */
    uint8_t  ttl;           /* time to live */
    uint8_t  protocol;      /* IPPROTO_TCP = 6 */
    uint16_t check;         /* header checksum */
    uint32_t saddr;         /* source IP (network byte order) */
    uint32_t daddr;         /* destination IP (network byte order) */
} __attribute__((packed));

#define IPPROTO_TCP  6
#define IP_HDR_LEN   20

/* ── TCP header (20 bytes, no options) ─────────────────────────────────────── */
struct tcp_hdr {
    uint16_t source;    /* source port (network byte order) */
    uint16_t dest;      /* destination port (network byte order) */
    uint32_t seq;       /* sequence number (network byte order) */
    uint32_t ack_seq;   /* acknowledgement number (network byte order) */
    uint8_t  doff_res;  /* data offset (4 bits) | reserved (4 bits) */
    uint8_t  flags;     /* control flags (see TCP_FLAG_* below) */
    uint16_t window;    /* receive window (network byte order) */
    uint16_t check;     /* checksum */
    uint16_t urg_ptr;   /* urgent pointer */
} __attribute__((packed));

#define TCP_HDR_LEN  20

/* TCP flag bits in tcp_hdr.flags */
#define TCP_FLAG_FIN  0x01
#define TCP_FLAG_SYN  0x02
#define TCP_FLAG_RST  0x04
#define TCP_FLAG_PSH  0x08
#define TCP_FLAG_ACK  0x10
#define TCP_FLAG_URG  0x20

/* ── TCP state machine ──────────────────────────────────────────────────────── */
enum tcp_state {
    TCP_CLOSED      = 0,
    TCP_LISTEN      = 1,
    TCP_SYN_RCVD    = 2,
    TCP_ESTABLISHED = 3,
    TCP_FIN_WAIT_1  = 4,
    TCP_FIN_WAIT_2  = 5,
    TCP_TIME_WAIT   = 6,
    TCP_CLOSE_WAIT  = 7,
    TCP_LAST_ACK    = 8,
};

/* ── Per-connection state ───────────────────────────────────────────────────── */
struct tcp_conn {
    enum tcp_state state;

    /* 4-tuple */
    uint32_t src_ip;    /* local IP (host byte order) */
    uint32_t dst_ip;    /* remote IP (host byte order) */
    uint16_t src_port;  /* local port (host byte order) */
    uint16_t dst_port;  /* remote port (host byte order) */

    /* Send-side sequence tracking */
    uint32_t snd_nxt;   /* next sequence number to send */
    uint32_t snd_una;   /* oldest unacknowledged sequence number */
    uint16_t snd_wnd;   /* remote receive window (host byte order) */

    /* Receive-side sequence tracking */
    uint32_t rcv_nxt;   /* next expected sequence number from remote */
    uint16_t rcv_wnd;   /* our receive window advertised to remote */

    /* Congestion control (v3) */
    uint32_t cwnd;          /* congestion window in bytes */
    uint32_t ssthresh;      /* slow-start threshold */
    int      dup_ack_count; /* consecutive duplicate ACK counter */

    /* Retransmit timer */
    struct timespec last_send;  /* time of last send (for RTO) */
    int             retransmitting;

    /* TIME_WAIT expiry */
    struct timespec time_wait_start;

    /* Send / receive buffers */
    uint8_t  send_buf[SEND_BUF_SIZE];
    int      send_len;
    uint8_t  recv_buf[RECV_BUF_SIZE];
    int      recv_len;

    int used; /* 1 if this slot is occupied */
};

/* Global connection table */
extern struct tcp_conn conn_table[MAX_CONNS];

/* ── Checksum ───────────────────────────────────────────────────────────────── */
uint16_t checksum(const void *data, int len);
uint16_t tcp_checksum(const struct ip_hdr *ip, const struct tcp_hdr *tcp,
                      const uint8_t *payload, int payload_len);

/* ── Packet I/O ─────────────────────────────────────────────────────────────── */
int  open_tun(const char *ifname);
void send_packet(int tun_fd, struct tcp_conn *conn, uint8_t flags,
                 const uint8_t *payload, int payload_len);

/* ── Connection management ─────────────────────────────────────────────────── */
struct tcp_conn *find_conn(uint32_t src_ip, uint32_t dst_ip,
                           uint16_t src_port, uint16_t dst_port);
struct tcp_conn *new_conn(void);
void             free_conn(struct tcp_conn *conn);

/* ── Millisecond helpers ────────────────────────────────────────────────────── */
long ms_elapsed(const struct timespec *since);

#endif /* TCP_H */
