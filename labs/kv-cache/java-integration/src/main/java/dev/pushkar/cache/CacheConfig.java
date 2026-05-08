package dev.pushkar.cache;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.cache.RedisCacheConfiguration;
import org.springframework.data.redis.cache.RedisCacheManager;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.connection.RedisStandaloneConfiguration;
import org.springframework.data.redis.connection.jedis.JedisClientConfiguration;
import org.springframework.data.redis.connection.jedis.JedisConnectionFactory;

import redis.clients.jedis.JedisPoolConfig;

import java.time.Duration;

/**
 * Spring configuration that wires Jedis 5.x to our Rust kv-cache server.
 *
 * <p>The key point: {@link JedisConnectionFactory} is told to connect to
 * {@code localhost:6380} — our Rust server, not a real Redis. Jedis has no
 * idea. The RESP2 protocol is simple enough that any compliant server is
 * completely transparent to the client.
 *
 * <p>Spring Cache abstraction ({@code @Cacheable}, {@code @CachePut},
 * {@code @CacheEvict}) sits above this layer. {@link OrderService} uses those
 * annotations without importing a single Jedis class. Swap
 * {@link RedisCacheManager} for {@code CaffeineCacheManager} and nothing in
 * {@link OrderService} changes. That is the entire value of the abstraction.
 */
@Configuration
@EnableCaching
@EnableConfigurationProperties(CacheProperties.class)
public class CacheConfig {

    private final CacheProperties props;

    public CacheConfig(CacheProperties props) {
        this.props = props;
    }

    /**
     * JedisPoolConfig — controls the connection pool.
     *
     * <p>maxTotal (maxActive): maximum connections checked out at once.
     * Requests block when the pool is exhausted.
     * maxIdle: how many idle connections to keep warm (avoids reconnect latency).
     */
    @Bean
    public JedisPoolConfig jedisPoolConfig() {
        var config = new JedisPoolConfig();
        config.setMaxTotal(props.pool().maxActive());
        config.setMaxIdle(props.pool().maxIdle());
        config.setMinIdle(1);
        config.setTestOnBorrow(true);        // validate connection before checkout
        config.setTestWhileIdle(true);       // evict stale idle connections
        return config;
    }

    /**
     * JedisConnectionFactory — points Jedis at our Rust server on port 6380.
     *
     * <p>Note the explicit Jedis client configuration: this is how you tell
     * Spring Data Redis to use Jedis instead of the default Lettuce client.
     */
    @Bean
    public RedisConnectionFactory redisConnectionFactory(JedisPoolConfig poolConfig) {
        var standalone = new RedisStandaloneConfiguration(props.host(), props.port());

        var clientConfig = JedisClientConfiguration.builder()
                .connectTimeout(props.pool().timeout())
                .readTimeout(props.pool().timeout())
                .usePooling()
                .poolConfig(poolConfig)
                .build();

        return new JedisConnectionFactory(standalone, clientConfig);
    }

    /**
     * RedisCacheManager — the Spring Cache implementation backed by Redis.
     *
     * <p>Default TTL of 5 minutes applies to all caches unless overridden.
     * Key prefix {@code "lab::"} namespaces our keys so they don't collide
     * with other apps sharing the same Redis/kv-cache instance.
     *
     * <p>disableCachingNullValues() prevents null results from being stored,
     * which avoids "cache poisoning" when a downstream service returns null
     * for a transient error.
     */
    @Bean
    public RedisCacheManager cacheManager(RedisConnectionFactory connectionFactory) {
        var defaultConfig = RedisCacheConfiguration.defaultCacheConfig()
                .entryTtl(Duration.ofMinutes(5))
                .prefixCacheNameWith("lab::")
                .disableCachingNullValues();

        return RedisCacheManager.builder(connectionFactory)
                .cacheDefaults(defaultConfig)
                .build();
    }
}
