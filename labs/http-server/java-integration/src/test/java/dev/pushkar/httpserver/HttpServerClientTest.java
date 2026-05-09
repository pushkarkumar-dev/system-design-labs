package dev.pushkar.httpserver;

import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import reactor.test.StepVerifier;

import java.io.IOException;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Unit tests for {@link HttpServerClient} using MockWebServer.
 *
 * <p>MockWebServer stands in for the real Go HTTP server. It lets us control
 * responses and inspect requests without starting any external process.
 * Each test uses a fresh MockWebServer instance to avoid state leaks.
 */
class HttpServerClientTest {

    private MockWebServer mockWebServer;
    private HttpServerClient client;

    @BeforeEach
    void setUp() throws IOException {
        mockWebServer = new MockWebServer();
        mockWebServer.start();
        client = new HttpServerClient(mockWebServer.url("/").toString());
    }

    @AfterEach
    void tearDown() throws IOException {
        mockWebServer.shutdown();
    }

    /**
     * Test 1: GET returns the response body when the server responds 200.
     */
    @Test
    void get_returnsBody_on200() {
        mockWebServer.enqueue(new MockResponse()
                .setResponseCode(200)
                .setBody("Hello from your hand-rolled HTTP server!\n"));

        StepVerifier.create(client.get("/"))
                .assertNext(body -> assertThat(body).contains("hand-rolled"))
                .verifyComplete();
    }

    /**
     * Test 2: POST sends the body and returns the transformed response.
     */
    @Test
    void post_sendsBodyAndReturnsResponse() throws InterruptedException {
        mockWebServer.enqueue(new MockResponse()
                .setResponseCode(200)
                .setBody("HELLO WORLD"));

        StepVerifier.create(client.post("/uppercase", "hello world"))
                .assertNext(body -> assertThat(body).isEqualTo("HELLO WORLD"))
                .verifyComplete();

        RecordedRequest request = mockWebServer.takeRequest();
        assertThat(request.getMethod()).isEqualTo("POST");
        assertThat(request.getPath()).isEqualTo("/uppercase");
        assertThat(request.getBody().readUtf8()).isEqualTo("hello world");
    }

    /**
     * Test 3: A 404 response is surfaced as an error in the reactive pipeline.
     */
    @Test
    void get_surfacesError_on404() {
        mockWebServer.enqueue(new MockResponse()
                .setResponseCode(404)
                .setBody("Not Found"));

        StepVerifier.create(client.get("/missing"))
                .expectErrorSatisfies(ex ->
                        assertThat(ex.getMessage()).contains("404"))
                .verify();
    }

    /**
     * Test 4: Connection reuse — a single MockWebServer handles 10 sequential
     * requests. Verifies that the WebClient does not fail across multiple calls.
     */
    @Test
    void get_handlesMultipleSequentialRequests() {
        for (int i = 0; i < 10; i++) {
            mockWebServer.enqueue(new MockResponse()
                    .setResponseCode(200)
                    .setBody("response-" + i));
        }

        for (int i = 0; i < 10; i++) {
            int index = i;
            StepVerifier.create(client.get("/"))
                    .assertNext(body -> assertThat(body).startsWith("response-"))
                    .verifyComplete();
        }

        assertThat(mockWebServer.getRequestCount()).isEqualTo(10);
    }

    /**
     * Test 5: Chunked response is decoded correctly by the HTTP client.
     * The Transfer-Encoding: chunked header is set; WebClient reads the
     * assembled body transparently.
     */
    @Test
    void get_decodesChunkedResponse() {
        mockWebServer.enqueue(new MockResponse()
                .setResponseCode(200)
                .setChunkedBody("hello from chunked encoding\n", 5));

        StepVerifier.create(client.get("/chunked"))
                .assertNext(body -> {
                    assertThat(body).isEqualTo("hello from chunked encoding\n");
                })
                .verifyComplete();
    }
}
