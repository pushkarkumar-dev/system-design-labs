/*
 * v0_blocking.c — blocking single-thread server
 *
 * Architecture:
 *   socket() → bind() → listen(BACKLOG) → loop: accept() → recv() → send() → close()
 *
 * One connection is fully processed before the next accept() call.
 * Every other client waits in the kernel's accept queue.
 *
 * Teaches:
 *   - The two-queue model: SYN queue (half-open) and accept queue (fully established)
 *   - listen(backlog) sizes the accept queue
 *   - When accept queue is full the kernel silently drops the ACK; client retransmits after RTO
 */

#include <arpa/inet.h>
#include <errno.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>

#define PORT    8080
#define BACKLOG 128   /* accept queue depth — try 1 with wrk to see drops  */
#define BUF_SZ  4096

static volatile int running = 1;

static void handle_sigint(int sig) {
    (void)sig;
    running = 0;
}

/*
 * print_queue_stats() — read /proc/net/tcp to show per-socket recv-Q / send-Q.
 * recv-Q on a listening socket = number of connections waiting in accept queue.
 */
static void print_queue_stats(int port) {
    FILE *f = fopen("/proc/net/tcp", "r");
    if (!f) return;

    char line[256];
    /* skip header */
    if (!fgets(line, sizeof(line), f)) { fclose(f); return; }

    unsigned local_addr, local_port, state;
    unsigned recv_q, send_q;

    printf("[queue-stats] port=%d\n", port);
    while (fgets(line, sizeof(line), f)) {
        if (sscanf(line, " %*d: %x:%x %*x:%*x %x %x:%x",
                   &local_addr, &local_port, &state, &recv_q, &send_q) == 5) {
            if (local_port == (unsigned)port) {
                /* state 0x0A = LISTEN */
                printf("  state=0x%02x recv-Q=%u (accept queue depth) send-Q=%u\n",
                       state, recv_q, send_q);
            }
        }
    }
    fclose(f);
}

/* Minimal HTTP/1.1 response — enough for curl to succeed */
static const char *RESPONSE =
    "HTTP/1.1 200 OK\r\n"
    "Content-Type: text/plain\r\n"
    "Content-Length: 6\r\n"
    "Connection: close\r\n"
    "\r\n"
    "hello\n";

static void handle_connection(int client_fd, struct sockaddr_in *addr) {
    char buf[BUF_SZ];
    char ip[INET_ADDRSTRLEN];
    inet_ntop(AF_INET, &addr->sin_addr, ip, sizeof(ip));

    ssize_t n = recv(client_fd, buf, sizeof(buf) - 1, 0);
    if (n <= 0) {
        close(client_fd);
        return;
    }
    buf[n] = '\0';

    /* Extract request line for logging */
    char method[16] = {0}, path[128] = {0};
    sscanf(buf, "%15s %127s", method, path);
    printf("[v0] conn from %s:%d  %s %s\n",
           ip, ntohs(addr->sin_port), method, path);

    send(client_fd, RESPONSE, strlen(RESPONSE), 0);
    close(client_fd);
}

int main(void) {
    signal(SIGINT, handle_sigint);

    int server_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (server_fd < 0) { perror("socket"); return 1; }

    int opt = 1;
    setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));

    struct sockaddr_in addr = {
        .sin_family      = AF_INET,
        .sin_port        = htons(PORT),
        .sin_addr.s_addr = INADDR_ANY,
    };

    if (bind(server_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        perror("bind"); close(server_fd); return 1;
    }

    /*
     * listen(fd, BACKLOG) sizes the accept queue.
     * The kernel caps this at /proc/sys/net/core/somaxconn (default 4096 on modern Linux).
     * When the queue is full and a new ACK arrives, the kernel drops it silently.
     * The client will retransmit after its RTO (typically 1 second on the first retry).
     */
    if (listen(server_fd, BACKLOG) < 0) {
        perror("listen"); close(server_fd); return 1;
    }

    printf("[v0] blocking server on port %d (backlog=%d)\n", PORT, BACKLOG);
    printf("     accept queue stats (requires /proc/net/tcp):\n");
    print_queue_stats(PORT);

    while (running) {
        struct sockaddr_in client_addr;
        socklen_t client_len = sizeof(client_addr);

        int client_fd = accept(server_fd, (struct sockaddr *)&client_addr, &client_len);
        if (client_fd < 0) {
            if (errno == EINTR) break;
            perror("accept");
            continue;
        }

        /*
         * KEY LESSON: we fully process the connection here before looping back.
         * During handle_connection(), every new incoming connection waits in
         * the kernel accept queue.  If the queue fills, new connections are dropped.
         */
        handle_connection(client_fd, &client_addr);
    }

    printf("\n[v0] shutting down\n");
    close(server_fd);
    return 0;
}
