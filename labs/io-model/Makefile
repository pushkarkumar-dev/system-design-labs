CC = gcc
CFLAGS = -O2 -Wall -Wextra -g

all: v0_blocking v1_threaded v2_epoll v3_io_uring

v0_blocking: src/v0_blocking.c
	$(CC) $(CFLAGS) -o $@ $<

v1_threaded: src/v1_threaded.c
	$(CC) $(CFLAGS) -o $@ $< -lpthread

v2_epoll: src/v2_epoll.c
	$(CC) $(CFLAGS) -o $@ $<

v3_io_uring: src/v3_io_uring.c
	$(CC) $(CFLAGS) -o $@ $< -luring

clean:
	rm -f v0_blocking v1_threaded v2_epoll v3_io_uring

.PHONY: all clean
