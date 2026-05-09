package com.labs.tcp;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.net.InetSocketAddress;
import java.nio.ByteBuffer;
import java.nio.channels.SocketChannel;
import java.nio.charset.StandardCharsets;
import java.time.Duration;

/**
 * TcpClient — connects to the userspace TCP echo server using Java NIO.
 *
 * Each call to roundTrip() opens a new connection, sends a message, reads
 * the echo, measures latency, and closes the channel. This mirrors what the
 * C echo server handles: one connect → data → FIN per call.
 *
 * Java NIO (java.nio.channels) chosen deliberately because:
 *   1. SocketChannel exposes the connection lifecycle explicitly (connect /
 *      finishConnect / read / write) — closer to the raw TCP operations.
 *   2. It avoids the abstraction of java.net.Socket which hides the SYN/ACK
 *      handshake behind a blocking connect() call with no timing visibility.
 *
 * Approximately 55 lines of logic (excluding Javadoc and blank lines).
 */
@Component
public class TcpClient {

    private final String host;
    private final int    port;

    public TcpClient(
            @Value("${tcp.echo-host:10.0.0.2}") String host,
            @Value("${tcp.echo-port:8080}")      int    port) {
        this.host = host;
        this.port = port;
    }

    /**
     * Send a message to the echo server, wait for the echo, return timing.
     *
     * @param message the string to send (UTF-8)
     * @return RoundTripResult with the echoed string and total latency
     * @throws Exception if the connection fails (server not running, route missing)
     */
    public RoundTripResult roundTrip(String message) throws Exception {
        long connectStart = System.nanoTime();

        // SocketChannel.open() + connect() sends the SYN.
        // The OS 3-way handshake (SYN → SYN-ACK → ACK) happens inside connect().
        // With our TUN stack that's ~1.2ms; with the kernel loopback it's ~0.08ms.
        try (SocketChannel channel = SocketChannel.open()) {
            channel.configureBlocking(true);
            channel.connect(new InetSocketAddress(host, port));

            long connectDone = System.nanoTime();
            Duration connectLatency = Duration.ofNanos(connectDone - connectStart);

            // write() triggers a send() syscall → the OS wraps bytes in a TCP segment.
            // Our stack will receive it via read(tun_fd), parse the header, and echo.
            byte[] bytes = message.getBytes(StandardCharsets.UTF_8);
            ByteBuffer sendBuf = ByteBuffer.wrap(bytes);
            while (sendBuf.hasRemaining()) {
                channel.write(sendBuf);
            }

            long writeDone = System.nanoTime();

            // read() blocks until our echo server sends the reply segment back.
            // The kernel receives it from tun_fd (via write(tun_fd, ...)),
            // delivers it to our SocketChannel buffer, and wakes this thread.
            ByteBuffer recvBuf = ByteBuffer.allocate(4096);
            int bytesRead = channel.read(recvBuf);

            long readDone = System.nanoTime();

            String echoed = bytesRead > 0
                    ? new String(recvBuf.array(), 0, bytesRead, StandardCharsets.UTF_8)
                    : "";

            return new RoundTripResult(
                    message,
                    echoed.trim(),
                    connectLatency,
                    Duration.ofNanos(readDone - writeDone),
                    Duration.ofNanos(readDone - connectStart)
            );
        }
    }

    /** Immutable result record — Java 16+ record syntax. */
    public record RoundTripResult(
            String   sent,
            String   echoed,
            Duration connectLatency,
            Duration dataLatency,
            Duration totalLatency
    ) {
        @Override public String toString() {
            return String.format(
                    "sent=%s echoed=%s connect=%dms data=%dms total=%dms",
                    sent, echoed,
                    connectLatency.toMillis(),
                    dataLatency.toMillis(),
                    totalLatency.toMillis());
        }
    }
}
