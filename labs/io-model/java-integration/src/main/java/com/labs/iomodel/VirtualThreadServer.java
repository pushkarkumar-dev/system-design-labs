package com.labs.iomodel;

import java.io.OutputStream;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;

/**
 * VirtualThreadServer — Java 21 Project Loom hybrid model.
 *
 * Write blocking code (accept → read → write → close).
 * The JVM schedules each virtual thread on a shared carrier thread pool
 * using park/unpark instead of real OS context switches.
 *
 * Key insight: virtual threads avoid the 8 MB stack problem.
 * A virtual thread's stack starts at a few hundred bytes and grows on demand.
 * 100,000 virtual threads consume ~100 MB, not 800 GB.
 *
 * This is a hybrid: the *code* looks like v0 (blocking), but the JVM
 * underneath uses the same carrier-thread scheduling that Netty's workers use.
 */
public class VirtualThreadServer {

    private static final int    PORT     = 9091;
    private static final byte[] RESPONSE =
        "HTTP/1.1 200 OK\r\nContent-Length: 6\r\nConnection: close\r\n\r\nhello\n"
            .getBytes(StandardCharsets.UTF_8);

    public static void main(String[] args) throws Exception {
        System.out.printf("[loom] virtual-thread server on port %d%n", PORT);
        System.out.println("[loom] each connection: Thread.ofVirtual().start(...)");
        System.out.println("[loom] JVM parks virtual thread on blocking I/O — no OS context switch");

        try (ServerSocket server = new ServerSocket(PORT)) {
            server.setReuseAddress(true);
            while (!Thread.currentThread().isInterrupted()) {
                Socket conn = server.accept();
                // Thread.ofVirtual() creates a virtual thread.
                // Blocking inside this lambda parks the virtual thread,
                // freeing the carrier thread for other virtual threads.
                Thread.ofVirtual().start(() -> handle(conn));
            }
        }
    }

    private static void handle(Socket conn) {
        try (conn) {
            // Blocking read — virtual thread parks here, carrier thread is freed.
            byte[] buf = conn.getInputStream().readNBytes(4096);
            if (buf.length == 0) return;
            OutputStream out = conn.getOutputStream();
            out.write(RESPONSE);
            out.flush();
        } catch (Exception e) {
            // Connection reset by client — expected under load tests.
        }
    }
}
