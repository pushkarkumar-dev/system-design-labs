package dev.pushkar.cdn;

import com.github.benmanes.caffeine.cache.Caffeine;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.cache.CacheManager;
import org.springframework.cache.caffeine.CaffeineCacheManager;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the CDN edge integration.
 *
 * <p>Provides {@link CdnClient} and a Caffeine-backed {@link CacheManager}
 * named {@code "cdn"} with configuration from {@link CdnProperties}.
 *
 * <p>The CDN cache hierarchy this wires up:
 * <pre>
 *   Request → Caffeine (@Cacheable in-JVM, L0) → CDN edge (L1/L2 LRU) → Origin
 * </pre>
 *
 * <p>{@code @ConditionalOnMissingBean} lets applications override either bean
 * by declaring their own — the standard Spring Boot customization contract.
 */
@AutoConfiguration
@EnableConfigurationProperties(CdnProperties.class)
public class CdnAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public CdnClient cdnClient(CdnProperties props) {
        return new CdnClient(props.edgeUrl());
    }

    /**
     * Caffeine-backed CacheManager named "cdn".
     *
     * <p>Used by {@code @Cacheable(cacheManager = "cdn")} in the service layer.
     * This is the in-JVM L0 cache: zero network RTT, heap-resident.
     *
     * <p>Separate from any other CacheManager in the application so that CDN
     * cache configuration (TTL, max-entries) is independent of other caches.
     */
    @Bean("cdnCacheManager")
    @ConditionalOnMissingBean(name = "cdnCacheManager")
    public CacheManager cdnCacheManager(CdnProperties props) {
        CaffeineCacheManager manager = new CaffeineCacheManager();
        manager.setCaffeine(Caffeine.newBuilder()
                .maximumSize(props.cache().maxEntries())
                .expireAfterWrite(props.cache().ttl())
                .recordStats());
        return manager;
    }
}
