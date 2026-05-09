package dev.pushkar.discovery;

/**
 * SpringCloudDiscoveryComparison — side-by-side commentary.
 *
 * <p>This class is NOT instantiated at runtime. It shows how the same
 * service discovery pattern looks in Spring Cloud (Eureka/Consul) so the
 * reader can see exactly what Spring Cloud is doing under the hood.
 *
 * <p><b>What our Go registry does:</b>
 * <pre>
 *   // Java client calling the Go registry directly:
 *   DiscoveryClient client = new DiscoveryClient("http://localhost:8080");
 *   List&lt;DiscoveryClient.ServiceInstance&gt; instances =
 *       client.getInstances("payment-service");
 *   // Pick one with round-robin and call it:
 *   var endpoint = instances.get(roundRobinIndex % instances.size());
 *   String url = "http://" + endpoint.address() + "/pay";
 * </pre>
 *
 * <p><b>What Spring Cloud does with Eureka:</b>
 * <pre>
 *   {@literal @}EnableDiscoveryClient       // activates EurekaDiscoveryClient
 *   {@literal @}SpringBootApplication
 *   public class PaymentConsumer { ... }
 *
 *   // Spring Cloud injects a LoadBalancerInterceptor into the RestTemplate.
 *   // That interceptor calls DiscoveryClient.getInstances() before every request
 *   // and applies a load-balancing algorithm (round-robin by default).
 *   {@literal @}Bean {@literal @}LoadBalanced
 *   RestTemplate restTemplate() { return new RestTemplate(); }
 *
 *   // Usage — the service name is the URL host; Spring Cloud resolves it:
 *   String response = restTemplate.getForObject(
 *       "http://payment-service/pay", String.class);
 *
 *   // Equivalent explicit call (what LoadBalancerInterceptor does internally):
 *   org.springframework.cloud.client.discovery.DiscoveryClient springDc = ...;
 *   List&lt;ServiceInstance&gt; instances = springDc.getInstances("payment-service");
 *   // instances.get(0).getUri() → http://10.0.0.3:8080
 * </pre>
 *
 * <p><b>Key differences:</b>
 * <ol>
 *   <li><b>Registry location</b>: our Go registry is a single process. Eureka
 *       replicates across a cluster of Eureka servers with eventual consistency.
 *       Consul uses Raft for stronger consistency guarantees.
 *   <li><b>Active health checks</b>: Spring Cloud Consul runs active TCP/HTTP
 *       probes from the registry server. We rely on client heartbeats only.
 *   <li><b>Integration depth</b>: Spring Cloud's {@code @LoadBalanced} annotation
 *       hooks into every HTTP call transparently. Our client requires explicit
 *       {@code getInstances()} calls. The Spring Cloud approach is convenient
 *       but hides what is happening; ours makes it visible.
 *   <li><b>Caching</b>: Spring Cloud LoadBalancer maintains a local cache
 *       refreshed on a schedule (default 35 seconds). Our v2 ServiceClient
 *       uses a Watch-driven cache that updates within milliseconds of a change.
 * </ol>
 */
public final class SpringCloudDiscoveryComparison {
    private SpringCloudDiscoveryComparison() {}

    /*
     * pom.xml dependencies for full Spring Cloud Eureka integration (not used
     * in this project — shown for comparison):
     *
     * <dependency>
     *   <groupId>org.springframework.cloud</groupId>
     *   <artifactId>spring-cloud-starter-netflix-eureka-client</artifactId>
     *   <version>4.1.3</version>
     * </dependency>
     * <dependency>
     *   <groupId>org.springframework.cloud</groupId>
     *   <artifactId>spring-cloud-starter-loadbalancer</artifactId>
     *   <version>4.1.4</version>
     * </dependency>
     *
     * application.yml for Eureka:
     *   eureka:
     *     client:
     *       service-url:
     *         defaultZone: http://eureka-server:8761/eureka/
     *     instance:
     *       prefer-ip-address: true
     */
}
