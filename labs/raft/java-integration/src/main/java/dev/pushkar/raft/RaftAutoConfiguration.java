package dev.pushkar.raft;

import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.List;

/**
 * Auto-configuration for the Raft cluster client.
 *
 * <p>Creates one {@link RaftClient} per configured node URL, then wires them
 * into a {@link RaftClusterService}.  Any bean already declared in the
 * application context takes precedence (via {@link ConditionalOnMissingBean}).
 */
@Configuration
@EnableConfigurationProperties(RaftProperties.class)
public class RaftAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public List<RaftClient> raftClients(RaftProperties props) {
        return props.nodeUrls().stream()
                .map(RaftClient::new)
                .toList();
    }

    @Bean
    @ConditionalOnMissingBean
    public RaftClusterService raftClusterService(List<RaftClient> clients, RaftProperties props) {
        return new RaftClusterService(clients, props);
    }
}
