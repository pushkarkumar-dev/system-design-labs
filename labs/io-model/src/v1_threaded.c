/*
 * v1_threaded.c — thread-per-connection server
 *
 * Architecture:
 *   accept loop → pthread_create per connection → each thread handles one client
 *
 * Teaches:
 *   - Thread stacks default to 8 MB — 10k threads = 80 GB virtual memory
 *   - Context switch overhead compounds at high thread counts
 *   - Thread pool bounds memory but makes the pool queue the new bottleneck
 *
 * Compile: gcc -O2 -Wall -o v1_threaded v1_threaded.c -lpthread
 */

#include <arpa/inet.h>
#include <errno.h>
#include <netinet/in.h>
#include <pthread.h>
#include <semaphore.h>
#include <signal.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <unistd.h>

#define PORT        8080
#define BACKLOG     512
#define BUF_SZ      4096

/*
 * MAX_THREADS caps the thread pool.
 * Without a cap: 10k connections × 8 MB stack = 80 GB virtual memory.
 * With a cap: the semaphore queue becomes the new backpressure mechanism.
 */
#define MAX_THREADS 1000

static volatile int      running = 1;
static atomic_int        active_threads = 0;
static sem_t             thread_slots;   /* counts available thread slots */

static void handle_sigint(int sig) {
    (void)sig;
    running = 0;
}

/* Per-connection argument passed to each thread */
typedef struct {
    int               fd;
    struct sockaddr_in addr;
} conn_arg_t;

static const char *RESPONSE =
    "HTTP/1.1 200 OK\r\n"
    "Content-Type: text/plain\r\n"
    "Content-Length: 6\r\n"
    "Connection: close\r\n"
    "\r\n"
    "hello\n";

static void *connection_thread(void *arg) {
    conn_arg_t *conn = (conn_arg_t *)arg;
    char buf[BUF_SZ];
    char ip[INET_ADDRSTRLEN];

    inet_ntop(AF_INET, &conn->addr.sin_addr, ip, sizeof(ip));

    ssize_t n = recv(conn->fd, buf, sizeof(buf) - 1, 0);
    if (n > 0) {
        buf[n] = '\0';
        char method[16] = {0}, path[128] = {0};
        sscanf(buf, "%15s %127s", method, path);
        int current = atomic_load(&active_threads);
        printf("[v1] thread %s:%d  %s %s  [active=%d/%d]\n",
               ip, ntohs(conn->addr.sin_port),
               method, path, current, MAX_THREADS);
        send(conn->fd, RESPONSE, strlen(RESPONSE), 0);
    }

    close(conn->fd);
    free(conn);

    atomic_fetch_sub(&active_threads, 1);
    sem_post(&thread_slots);   /* release slot back to pool */
    return NULL;
}

/*
 * show_stack_size() — print the default thread stack size.
 * On Linux the default is 8 MB per thread.
 * 10,000 threads × 8 MB = 80 GB virtual address space.
 * This is the C10k wall: memory, not CPU.
 */
static void show_stack_size(void) {
    pthread_attr_t attr;
    pthread_attr_init(&attr);
    size_t stack_size = 0;
    pthread_attr_getstacksize(&attr, &stack_size);
    pthread_attr_destroy(&attr);
    printf("[v1] default thread stack size: %zu MB (%zu bytes)\n",
           stack_size / (1024 * 1024), stack_size);
    printf("[v1] 10,000 threads would consume: %zu GB virtual memory\n",
           (stack_size * 10000) / (1024 * 1024 * 1024));
}

int main(void) {
    signal(SIGINT, handle_sigint);

    show_stack_size();

    /* Initialise semaphore to MAX_THREADS available slots */
    if (sem_init(&thread_slots, 0, MAX_THREADS) != 0) {
        perror("sem_init"); return 1;
    }

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
    if (listen(server_fd, BACKLOG) < 0) {
        perror("listen"); close(server_fd); return 1;
    }

    printf("[v1] thread-per-connection server on port %d (max_threads=%d)\n",
           PORT, MAX_THREADS);

    while (running) {
        struct sockaddr_in client_addr;
        socklen_t client_len = sizeof(client_addr);

        int client_fd = accept(server_fd,
                               (struct sockaddr *)&client_addr, &client_len);
        if (client_fd < 0) {
            if (errno == EINTR) break;
            perror("accept");
            continue;
        }

        /*
         * sem_wait() blocks when MAX_THREADS threads are active.
         * This is the thread pool's backpressure mechanism.
         * The connection sits in the accept queue until a slot is free.
         * Observation: the pool queue is now the new bottleneck — we traded
         * OOM for latency.
         */
        sem_wait(&thread_slots);
        atomic_fetch_add(&active_threads, 1);

        conn_arg_t *conn = malloc(sizeof(*conn));
        if (!conn) {
            close(client_fd);
            atomic_fetch_sub(&active_threads, 1);
            sem_post(&thread_slots);
            continue;
        }
        conn->fd   = client_fd;
        conn->addr = client_addr;

        pthread_t tid;
        pthread_attr_t tattr;
        pthread_attr_init(&tattr);
        pthread_attr_setdetachstate(&tattr, PTHREAD_CREATE_DETACHED);

        /*
         * Demonstrate the stack size knob.
         * Reducing to 256 KB allows ~32k threads in 8 GB but limits recursion depth.
         * Default 8 MB is safe for deep call stacks but prohibitively large at scale.
         */
        pthread_attr_setstacksize(&tattr, 8 * 1024 * 1024);   /* 8 MB default */

        if (pthread_create(&tid, &tattr, connection_thread, conn) != 0) {
            perror("pthread_create");
            close(client_fd);
            free(conn);
            atomic_fetch_sub(&active_threads, 1);
            sem_post(&thread_slots);
        }
        pthread_attr_destroy(&tattr);
    }

    printf("\n[v1] shutting down\n");
    close(server_fd);
    sem_destroy(&thread_slots);
    return 0;
}
