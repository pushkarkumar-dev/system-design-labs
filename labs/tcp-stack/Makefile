# Makefile for labs/tcp-stack
#
# Requires: Linux (TUN/TAP is Linux-only), gcc, root privileges at runtime
#
# Usage:
#   make all   — build all four stages
#   make v0    — TUN device + packet parsing
#   make v1    — 3-way handshake
#   make v2    — reliable delivery + retransmission
#   make v3    — congestion control (slow start + CA + fast retransmit)
#   make clean — remove binaries

CC      = gcc
CFLAGS  = -O2 -Wall -Wextra -std=c11 -I./src
LDFLAGS =

SRCS_V0 = src/v0_ip.c
SRCS_V1 = src/v1_handshake.c
SRCS_V2 = src/v2_reliable.c
SRCS_V3 = src/v3_congestion.c

.PHONY: all v0 v1 v2 v3 clean

all: v0 v1 v2 v3

v0: $(SRCS_V0) src/tcp.h
	$(CC) $(CFLAGS) -o v0_ip $(SRCS_V0) $(LDFLAGS)
	@echo "Built v0_ip — run with: sudo ./v0_ip"

v1: $(SRCS_V1) src/tcp.h
	$(CC) $(CFLAGS) -o v1_handshake $(SRCS_V1) $(LDFLAGS)
	@echo "Built v1_handshake — run with: sudo ./v1_handshake"

v2: $(SRCS_V2) src/tcp.h
	$(CC) $(CFLAGS) -o v2_reliable $(SRCS_V2) $(LDFLAGS)
	@echo "Built v2_reliable — run with: sudo ./v2_reliable"

v3: $(SRCS_V3) src/tcp.h
	$(CC) $(CFLAGS) -o v3_congestion $(SRCS_V3) $(LDFLAGS)
	@echo "Built v3_congestion — run with: sudo ./v3_congestion"

clean:
	rm -f v0_ip v1_handshake v2_reliable v3_congestion
