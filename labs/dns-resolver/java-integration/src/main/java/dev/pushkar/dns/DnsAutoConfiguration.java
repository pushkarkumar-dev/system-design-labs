package dev.pushkar.dns;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Spring auto-configuration for the DNS lab beans.
 *
 * <p>Wires up:
 * <ul>
 *   <li>{@link DnsAdminClient} — HTTP client for the Go resolver's admin API
 *   <li>{@link DnsJavaComparison} — side-by-side dnsjava vs our resolver
 * </ul>
 */
@Configuration
@EnableConfigurationProperties(DnsProperties.class)
public class DnsAutoConfiguration {

    @Bean
    public DnsAdminClient dnsAdminClient(DnsProperties props) {
        return new DnsAdminClient(props.adminUrl());
    }

    @Bean
    public DnsJavaComparison dnsJavaComparison(DnsProperties props) {
        return new DnsJavaComparison(props.resolverHost(), props.resolverPort());
    }
}
