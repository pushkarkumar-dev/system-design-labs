package dev.pushkar.dns;

import org.xbill.DNS.ARecord;
import org.xbill.DNS.Lookup;
import org.xbill.DNS.Resolver;
import org.xbill.DNS.SimpleResolver;
import org.xbill.DNS.Type;

import java.net.InetAddress;
import java.util.ArrayList;
import java.util.List;

/**
 * Side-by-side comparison of our Go resolver vs the dnsjava system resolver.
 *
 * <p>Our resolver:
 * <ul>
 *   <li>Walks the delegation chain step by step (visible in the Go logs)
 *   <li>Has a TTL-based cache you can inspect via the admin API
 *   <li>Returns NXDOMAIN as a typed error with negative TTL caching
 * </ul>
 *
 * <p>dnsjava with system resolver (8.8.8.8):
 * <ul>
 *   <li>Hides the delegation tree — one call, one answer
 *   <li>Supports DNSSEC validation, EDNS0, TSIG, zone transfers
 *   <li>Thread-safe, battle-tested, handles edge cases we don't
 * </ul>
 *
 * <p>Both use the same API ({@link Lookup}) — only the {@link Resolver} target differs.
 */
public class DnsJavaComparison {

    private final String ourResolverHost;
    private final int ourResolverPort;

    public DnsJavaComparison(String resolverHost, int resolverPort) {
        this.ourResolverHost = resolverHost;
        this.ourResolverPort = resolverPort;
    }

    /**
     * Resolve a domain A record using our Go resolver.
     * dnsjava sends the query to 127.0.0.1:5300 (our server).
     */
    public List<String> resolveWithOurResolver(String domain) throws Exception {
        Resolver resolver = new SimpleResolver(ourResolverHost);
        resolver.setPort(ourResolverPort);
        resolver.setTimeout(java.time.Duration.ofSeconds(5));

        Lookup lookup = new Lookup(domain, Type.A);
        lookup.setResolver(resolver);

        var records = lookup.run();
        if (records == null) {
            return List.of(); // NXDOMAIN or timeout
        }

        List<String> addresses = new ArrayList<>();
        for (var record : records) {
            if (record instanceof ARecord a) {
                addresses.add(a.getAddress().getHostAddress());
            }
        }
        return addresses;
    }

    /**
     * Resolve using the system resolver (defaults to OS-configured DNS, typically 8.8.8.8 or similar).
     * This goes through dnsjava's full stack — EDNS0, DNSSEC-aware, proper CNAME handling.
     */
    public List<String> resolveWithSystemResolver(String domain) throws Exception {
        // Lookup with no explicit resolver → uses OS-configured resolver
        Lookup lookup = new Lookup(domain, Type.A);

        var records = lookup.run();
        if (records == null) {
            return List.of();
        }

        List<String> addresses = new ArrayList<>();
        for (var record : records) {
            if (record instanceof ARecord a) {
                addresses.add(a.getAddress().getHostAddress());
            }
        }
        return addresses;
    }

    /**
     * Resolve using the JDK's built-in resolver (InetAddress).
     * This is what Java code uses by default when calling InetAddress.getByName().
     * It bypasses dnsjava entirely and goes straight to the OS resolver.
     */
    public String resolveWithJdkResolver(String domain) throws Exception {
        return InetAddress.getByName(domain).getHostAddress();
    }
}
