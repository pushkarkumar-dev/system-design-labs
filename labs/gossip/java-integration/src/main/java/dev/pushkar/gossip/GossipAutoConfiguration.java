package dev.pushkar.gossip;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.scheduling.annotation.EnableScheduling;

/**
 * Spring Boot auto-configuration for the gossip cluster integration.
 *
 * <p>Any Spring Boot application that includes this module and sets
 * {@code gossip.base-url} in {@code application.yml} gets ready-to-inject
 * {@link GossipClient} and {@link ClusterHealthService} beans with zero
 * extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the application can override
 * either bean by declaring its own — standard Spring Boot customization contract.
 * This is the same pattern Spring Cloud Consul uses for its {@code ConsulClient}.
 */
@AutoConfiguration
@EnableScheduling
@EnableConfigurationProperties(GossipProperties.class)
public class GossipAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public GossipClient gossipClient(GossipProperties props) {
        return new GossipClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public ClusterHealthService clusterHealthService(GossipClient client,
                                                     GossipProperties props) {
        return new ClusterHealthService(client, props);
    }
}
