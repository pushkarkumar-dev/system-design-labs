package dev.pushkar.ws;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration for the WebSocket gateway client.
 *
 * <p>Enables {@link WsGatewayProperties} so that {@code ws-gateway.*} keys
 * in {@code application.properties} are bound automatically.
 */
@Configuration
@EnableConfigurationProperties(WsGatewayProperties.class)
public class WsGatewayAutoConfiguration {
    // WsGatewayClient is constructed manually in the demo to allow
    // per-connection user IDs. A production auto-configuration would expose
    // a primary WsGatewayClient bean here.
}
