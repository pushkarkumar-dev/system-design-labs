package dev.pushkar.agent;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Unit tests for AgentClient and AgentProperties.
 * These tests do not require the Python server to be running.
 */
class AgentClientTest {

    @Test
    void agentPropertiesDefaults() {
        var props = new AgentProperties();
        assertEquals("http://localhost:8001", props.getBaseUrl());
        assertEquals("function", props.getDefaultMode());
    }

    @Test
    void agentClientConstructsWithoutError() {
        // Just verifies the RestClient builder doesn't throw on construction
        assertDoesNotThrow(() -> new AgentClient("http://localhost:8001"));
    }
}
