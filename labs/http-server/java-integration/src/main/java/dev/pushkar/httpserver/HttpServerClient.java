package dev.pushkar.httpserver;

import org.springframework.web.reactive.function.client.WebClient;
import org.springframework.web.reactive.function.client.WebClientResponseException;
import reactor.core.publisher.Mono;

/**
 * Reactive WebClient wrapper for the hand-rolled Go HTTP/1.1 server.
 *
 * <p>Why WebClient over RestTemplate?
 * RestTemplate is a blocking, thread-per-request model. Each in-flight request
 * occupies one thread. WebClient uses Reactor Netty under the hood — a single
 * event loop handles thousands of concurrent connections without blocking any
 * thread. The result: lower memory (no thread stacks) and better tail latency
 * under load.
 *
 * <p>Keep this class under 60 lines of logic; Spring wiring lives in
 * {@link HttpServerAutoConfiguration}.
 */
public class HttpServerClient {

    private final WebClient webClient;

    /**
     * Creates a client targeting the given base URL.
     *
     * @param baseUrl base URL of the Go server, e.g. {@code http://localhost:8080}
     */
    public HttpServerClient(String baseUrl) {
        this.webClient = WebClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", "text/plain")
                .build();
    }

    /**
     * Sends an HTTP GET to {@code path} and returns the response body as a String.
     * 4xx and 5xx responses are surfaced as {@link WebClientResponseException}.
     *
     * @param path request path, e.g. {@code /} or {@code /echo}
     * @return a cold {@link Mono} that emits the response body on subscription
     */
    public Mono<String> get(String path) {
        return webClient.get()
                .uri(path)
                .retrieve()
                .onStatus(
                        status -> status.is4xxClientError() || status.is5xxServerError(),
                        response -> response.createException().flatMap(Mono::error))
                .bodyToMono(String.class);
    }

    /**
     * Sends an HTTP POST to {@code path} with a plain-text body and returns the
     * response body as a String.
     * 4xx and 5xx responses are surfaced as {@link WebClientResponseException}.
     *
     * @param path  request path, e.g. {@code /uppercase}
     * @param body  request body text
     * @return a cold {@link Mono} that emits the response body on subscription
     */
    public Mono<String> post(String path, String body) {
        return webClient.post()
                .uri(path)
                .header("Content-Type", "text/plain")
                .bodyValue(body)
                .retrieve()
                .onStatus(
                        status -> status.is4xxClientError() || status.is5xxServerError(),
                        response -> response.createException().flatMap(Mono::error))
                .bodyToMono(String.class);
    }
}
