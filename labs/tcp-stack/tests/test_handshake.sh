#!/bin/bash
# test_handshake.sh — integration test for v1_handshake
#
# Requirements: Linux, root / sudo, nc (netcat), make
#
# What this tests:
#   1. Creates a tun0 TUN interface at 10.0.0.1/24
#   2. Runs v1_handshake in the background (listens on 10.0.0.2:8080)
#   3. Uses nc to send "hello" and verify the handshake completes
#   4. Checks the server printed "ESTABLISHED"
#
# Note: This is a Linux-only test. macOS and Windows do not have /dev/net/tun.

set -e

BINARY="./v1_handshake"
LOG=$(mktemp /tmp/handshake_XXXXXX.log)

cleanup() {
    kill %1 2>/dev/null || true
    sudo ip link set tun0 down 2>/dev/null || true
    sudo ip tuntap del mode tun tun0 2>/dev/null || true
    rm -f "$LOG"
}
trap cleanup EXIT

echo "=== TCP Handshake Integration Test ==="
echo ""

# Build if needed
if [ ! -f "$BINARY" ]; then
    echo "[setup] Building v1_handshake..."
    make v1
fi

# Tear down any leftover tun0 from a previous run
sudo ip link set tun0 down 2>/dev/null || true
sudo ip tuntap del mode tun tun0 2>/dev/null || true

echo "[setup] Starting v1_handshake in background..."
sudo "$BINARY" > "$LOG" 2>&1 &
SERVER_PID=$!

# Give the server time to open /dev/net/tun and bring up the interface
sleep 0.8

echo "[test] Sending TCP connection to 10.0.0.2:8080..."
# nc: -w 1 = 1-second timeout, -z = port scan (just connect, don't send data)
echo "hello tcp" | sudo nc 10.0.0.2 8080 -w 1 2>&1 || true

sleep 0.3

echo "[check] Server output:"
cat "$LOG"
echo ""

if grep -q "ESTABLISHED" "$LOG"; then
    echo "✓ PASS: 3-way handshake completed — ESTABLISHED printed"
    exit 0
else
    echo "✗ FAIL: ESTABLISHED not found in server output"
    exit 1
fi
