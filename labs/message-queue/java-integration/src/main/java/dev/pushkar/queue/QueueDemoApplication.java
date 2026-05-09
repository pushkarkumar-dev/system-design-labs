package dev.pushkar.queue;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.ConfigurableApplicationContext;

import java.util.List;

/**
 * Demonstrates the full send → receive → delete cycle against the Go
 * message-queue server, then drains the DLQ to show poison-pill handling.
 *
 * <p>Run with:
 * <pre>
 *   # Terminal 1: start the Go server
 *   cd labs/message-queue
 *   go run ./cmd/server --port 8080
 *
 *   # Terminal 2: run this demo
 *   cd labs/message-queue/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p><strong>The @JmsListener comparison</strong><br>
 * With real JMS (e.g., ActiveMQ via Spring Boot), you would write:
 * <pre>
 *   {@code @JmsListener(destination = "orders")}
 *   public void onOrder(String body) {
 *       processOrder(body);
 *       // JMS auto-acks when method returns normally.
 *       // If an exception is thrown, the message is redelivered.
 *   }
 * </pre>
 * The JMS container handles the polling loop, the ack/nack lifecycle, and
 * dead-letter routing after N failed deliveries — exactly what this demo
 * implements manually. Making it explicit teaches you what "at-least-once
 * delivery" really means: the server keeps the message visible until you
 * explicitly acknowledge (delete) it or the visibility timeout expires.
 */
@SpringBootApplication
@EnableConfigurationProperties(QueueProperties.class)
public class QueueDemoApplication {

    public static void main(String[] args) {
        ConfigurableApplicationContext ctx = SpringApplication.run(QueueDemoApplication.class, args);
        QueueClient client = ctx.getBean(QueueClient.class);
        QueueProperties props = ctx.getBean(QueueProperties.class);

        String queue = props.defaultQueueName();
        String dlqName = "dlq";

        System.out.println("=== Message Queue Spring Integration Demo ===\n");

        // ── Step 1: Send 100 messages ─────────────────────────────────────────
        System.out.printf("Sending 100 messages to queue '%s'...%n", queue);
        for (int i = 0; i < 100; i++) {
            client.sendMessage(queue, "order:" + i + ":user=" + (i % 10));
        }
        System.out.println("Sent 100 messages.\n");

        // ── Step 2: Receive in batches of 10 and delete ───────────────────────
        System.out.println("--- Processing in batches of 10 (at-least-once) ---");
        int processed = 0;
        int visTimeout = props.defaultVisibilityTimeoutSec();

        while (processed < 100) {
            List<QueueClient.QueueMessage> msgs =
                    client.receiveMessages(queue, props.maxMessages(), visTimeout);
            if (msgs.isEmpty()) break;

            for (QueueClient.QueueMessage msg : msgs) {
                // Simulate processing.
                System.out.printf("  Processing: %s (receiveCount=%d)%n",
                        msg.body(), msg.receiveCount());

                // Delete = acknowledge. Without this call, the message would
                // reappear after visibilityTimeoutSec — exactly like a JMS
                // session that hasn't been acknowledged.
                client.deleteMessage(queue, msg.receiptHandle());
                processed++;
            }
            System.out.printf("  Batch done. Total processed: %d%n", processed);
        }
        System.out.printf("%nProcessed %d messages.%n%n", processed);

        // ── Step 3: DLQ drain ─────────────────────────────────────────────────
        // The Go server was started without a DLQ, so we just show what the
        // pattern looks like. In production you would create main + DLQ queues
        // at startup and the server would auto-promote poison pills.
        System.out.println("--- DLQ drain pattern (would drain real DLQ if configured) ---");
        System.out.printf("""
                // To create a DLQ-backed queue with the Go server:
                //   POST /queues  {"name":"orders", "dlqName":"orders-dlq", "maxReceiveCount":3}
                //   POST /queues  {"name":"orders-dlq"}   // create DLQ first
                //
                // Then to drain the DLQ:
                List<QueueMessage> dead = client.receiveMessages("%s", 10, 60);
                dead.forEach(m -> {
                    log.error("DLQ message: {}", m.body());
                    // Inspect, alert, re-process manually, or discard.
                    client.deleteMessage("%s", m.receiptHandle());
                });
                %n""", dlqName, dlqName);

        // ── Step 4: The @JmsListener comparison ───────────────────────────────
        System.out.println("--- What @JmsListener hides from you ---");
        System.out.println("""
                // With Spring JMS + ActiveMQ:
                //   @JmsListener(destination = "orders")
                //   public void onOrder(String body) { processOrder(body); }
                //
                // Under the hood, DefaultMessageListenerContainer does:
                //   1. Poll MessageConsumer.receive() in a thread pool
                //   2. Open a JMS Session (= our visibility timeout window)
                //   3. Deliver the message to your @JmsListener method
                //   4. On success: session.commit() (= our DeleteMessage)
                //   5. On exception: session.rollback() (= message reappears)
                //   6. After N rollbacks: move to DLQ (configured via activemq.redeliveryPolicy)
                //
                // The SQS model (our queue) makes steps 2-6 explicit:
                //   - ReceiptHandle = the session handle
                //   - VisibilityTimeout = the session transaction window
                //   - DeleteMessage = the commit
                //   - Timeout expiry = the rollback
                //   - maxReceiveCount = the redelivery limit
                //
                // Making it explicit is not a step backward — it's what lets
                // you set different visibility timeouts per message type, extend
                // timeouts for long-running jobs, and drain DLQs independently.
                """);

        System.out.println("=== Demo complete ===");
        ctx.close();
    }
}
