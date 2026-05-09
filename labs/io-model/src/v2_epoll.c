/*
 * v2_epoll.c — epoll event loop, single-threaded, edge-triggered
 *
 * Architecture:
 *   Non-blocking server socket + epoll interest list.
 *   epoll_wait returns only ready FDs (O(1) vs poll/select O(n)).
 *   Edge-triggered (EPOLLET): fires once per state change → must drain loop.
 *
 * Teaches:
 *   - select()/poll() scan all N FDs every call; epoll maintains a kernel interest list
 *   - EPOLLET fires once per state change; EPOLLIN fires while data is available
 *   - One thread, 50k connections; slow handler blocks ALL other connections
 *
 * Compile: gcc -O2 -Wall -o v2_epoll v2_epoll.c
 */

#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>

#define PORT         8080
#define BACKLOG      512
#define MAX_EVENTS   1024
#define BUF_SZ       4096
#define MAX_CONNS    65536

static volatile int running = 1;

static void handle_sigint(int sig) {
    (void)sig;
    running = 0;
}

/* Per-connection state (kept so we can accumulate a partial request) */
typedef struct {
    int    fd;
    char   rbuf[BUF_SZ];
    int    rlen;
    int    active;
} conn_state_t;

static conn_state_t conn_table[MAX_CONNS];
static atomic_int   conn_count = 0;

static const char *RESPONSE =
    "HTTP/1.1 200 OK\r\n"
    "Content-Type: text/plain\r\n"
    "Content-Length: 6\r\n"
    "Connection: close\r\n"
    "\r\n"
    "hello\n";

/* Set a file descriptor to non-blocking mode */
static int set_nonblocking(int fd) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0) return -1;
    return fcntl(fd, F_SETFL, flags | O_NONBLOCK);
}

/* Register fd with epoll in edge-triggered mode */
static int epoll_add(int epfd, int fd, uint32_t events) {
    struct epoll_event ev = { .events = events, .data.fd = fd };
    return epoll_ctl(epfd, EPOLL_CTL_ADD, fd, &ev);
}

/* Remove fd from epoll (before close) */
static void epoll_del(int epfd, int fd) {
    epoll_ctl(epfd, EPOLL_CTL_DEL, fd, NULL);
}

static void close_conn(int epfd, int fd) {
    epoll_del(epfd, fd);
    if (fd >= 0 && fd < MAX_CONNS && conn_table[fd].active) {
        conn_table[fd].active = 0;
        atomic_fetch_sub(&conn_count, 1);
    }
    close(fd);
}

/*
 * handle_readable() — drain all available data in edge-triggered mode.
 *
 * With EPOLLET the kernel fires exactly ONCE per new data arrival.
 * If we only call recv() once and there is more data, we will never get
 * another notification for that data.  We must read in a loop until
 * recv() returns EAGAIN (no more data right now).
 */
static void handle_readable(int epfd, int fd) {
    conn_state_t *c = &conn_table[fd];

    while (1) {
        ssize_t n = recv(fd,
                         c->rbuf + c->rlen,
                         sizeof(c->rbuf) - 1 - c->rlen, 0);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                /* No more data right now — stop draining */
                break;
            }
            /* Real error */
            close_conn(epfd, fd);
            return;
        }
        if (n == 0) {
            /* Client closed connection */
            close_conn(epfd, fd);
            return;
        }
        c->rlen += (int)n;
        c->rbuf[c->rlen] = '\0';

        /* Check if we have a complete HTTP request (ends with \r\n\r\n) */
        if (strstr(c->rbuf, "\r\n\r\n")) {
            char method[16] = {0}, path[128] = {0};
            sscanf(c->rbuf, "%15s %127s", method, path);
            printf("[v2] fd=%-5d  %s %s  [conns=%d]\n",
                   fd, method, path, atomic_load(&conn_count));

            /* Write response — non-blocking, single send is fine for small payloads */
            send(fd, RESPONSE, strlen(RESPONSE), 0);
            close_conn(epfd, fd);
            return;
        }

        /* Partial request — keep the data, wait for more */
        if (c->rlen >= (int)sizeof(c->rbuf) - 1) {
            /* Buffer full without a complete request — reject */
            close_conn(epfd, fd);
            return;
        }
    }
}

int main(void) {
    signal(SIGINT, handle_sigint);

    memset(conn_table, 0, sizeof(conn_table));

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

    /* Non-blocking server socket — required for edge-triggered accept */
    set_nonblocking(server_fd);

    if (listen(server_fd, BACKLOG) < 0) {
        perror("listen"); close(server_fd); return 1;
    }

    /*
     * epoll_create1(0) creates the epoll file descriptor.
     * The kernel allocates an internal interest list (a red-black tree) and
     * a ready list.  Only FDs on the ready list are returned by epoll_wait.
     * This is the O(1) property: cost is proportional to ready events,
     * not to the total number of monitored FDs.
     */
    int epfd = epoll_create1(0);
    if (epfd < 0) { perror("epoll_create1"); close(server_fd); return 1; }

    /*
     * EPOLLET  = edge-triggered: fire once per state transition
     * EPOLLIN  = readable data available
     *
     * We use EPOLLET on the server socket too so that accept() must be
     * called in a loop until EAGAIN on each epoll_wait notification.
     */
    epoll_add(epfd, server_fd, EPOLLIN | EPOLLET);

    printf("[v2] epoll event loop on port %d (edge-triggered, EPOLLET)\n", PORT);
    printf("[v2] epoll vs poll/select:\n");
    printf("     poll/select: O(n) — kernel scans all N FDs every call\n");
    printf("     epoll:       O(1) — kernel returns only ready FDs\n\n");

    struct epoll_event events[MAX_EVENTS];

    while (running) {
        int n = epoll_wait(epfd, events, MAX_EVENTS, 500 /* ms timeout */);
        if (n < 0) {
            if (errno == EINTR) break;
            perror("epoll_wait");
            continue;
        }

        for (int i = 0; i < n; i++) {
            int fd = events[i].data.fd;

            if (fd == server_fd) {
                /*
                 * Server socket is readable: one or more new connections.
                 * With EPOLLET we must accept in a loop until EAGAIN.
                 */
                while (1) {
                    struct sockaddr_in client_addr;
                    socklen_t client_len = sizeof(client_addr);
                    int client_fd = accept(server_fd,
                                          (struct sockaddr *)&client_addr,
                                          &client_len);
                    if (client_fd < 0) {
                        if (errno == EAGAIN || errno == EWOULDBLOCK) break;
                        perror("accept");
                        break;
                    }

                    if (client_fd >= MAX_CONNS) {
                        /* Too many connections */
                        close(client_fd);
                        continue;
                    }

                    set_nonblocking(client_fd);

                    conn_table[client_fd].fd     = client_fd;
                    conn_table[client_fd].rlen   = 0;
                    conn_table[client_fd].active = 1;
                    atomic_fetch_add(&conn_count, 1);

                    epoll_add(epfd, client_fd, EPOLLIN | EPOLLET);
                }

            } else if (events[i].events & (EPOLLERR | EPOLLHUP)) {
                /* Error or hangup */
                close_conn(epfd, fd);

            } else if (events[i].events & EPOLLIN) {
                handle_readable(epfd, fd);
            }
        }
    }

    printf("\n[v2] shutting down  (peak connections tracked: O(1) readiness)\n");
    close(epfd);
    close(server_fd);
    return 0;
}
