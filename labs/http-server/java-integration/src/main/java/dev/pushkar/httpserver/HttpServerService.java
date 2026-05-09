package dev.pushkar.httpserver;

import org.springframework.stereotype.Service;
import reactor.core.publisher.Mono;

/**
 * Application-level service wrapping {@link HttpServerClient}.
 *
 * <p>This is where you would add retry logic, circuit breaking, or caching
 * on top of the raw WebClient calls. For this demo the logic is intentionally
 * minimal so the reactive plumbing stays visible.
 */
@Service
public class HttpServerService {

    private final HttpServerClient client;

    public HttpServerService(HttpServerClient client) {
        this.client = client;
    }

    /**
     * Fetches the page at the given path from the Go server.
     *
     * @param path request path (e.g. {@code /} or {@code /echo})
     * @return a cold {@link Mono} that emits the page body on subscription
     */
    public Mono<String> fetchPage(String path) {
        return client.get(path);
    }

    /**
     * POSTs {@code input} to the server's {@code /uppercase} endpoint and
     * returns the transformed result.
     *
     * <p>Demonstrates a POST over the reactive pipeline — note that nothing
     * executes until a subscriber calls {@code subscribe()} or {@code block()}.
     *
     * @param input text to be uppercased by the Go server
     * @return a cold {@link Mono} that emits the uppercased text on subscription
     */
    public Mono<String> transform(String input) {
        return client.post("/uppercase", input);
    }
}
