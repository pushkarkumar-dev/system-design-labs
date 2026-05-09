package dev.pushkar.crdt;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Unit tests for the Java GCounter implementation.
 * No Spring context needed — these test the pure data structure.
 */
class GCounterTest {

    @Test
    void increment_accumulatesValue() {
        GCounter g = new GCounter();
        g.increment("n1");
        g.increment("n1");
        g.increment("n2");

        assertThat(g.value()).isEqualTo(3L);
    }

    @Test
    void merge_takesMaxPerNode() {
        GCounter a = new GCounter();
        GCounter b = new GCounter();
        a.increment("n1");
        a.increment("n1"); // n1=2
        b.increment("n1"); // n1=1 on b — should not overwrite a's n1=2

        a.merge(b);
        assertThat(a.value()).isEqualTo(2L); // max(2,1) = 2, not 3
    }

    @Test
    void merge_isCommutative() {
        GCounter a = new GCounter();
        GCounter b = new GCounter();
        a.increment("n1");
        b.increment("n2");

        GCounter ab = a.copy();
        ab.merge(b);

        GCounter ba = b.copy();
        ba.merge(a);

        assertThat(ab.value()).isEqualTo(ba.value());
    }

    @Test
    void merge_isIdempotent() {
        GCounter a = new GCounter();
        a.increment("n1");
        a.increment("n1");
        long before = a.value();

        a.merge(a.copy());
        assertThat(a.value()).isEqualTo(before);
    }

    @Test
    void merge_isAssociative() {
        GCounter a = new GCounter();
        GCounter b = new GCounter();
        GCounter c = new GCounter();
        a.increment("n1");
        b.increment("n2");
        c.increment("n3");

        // (a merge b) merge c
        GCounter abc1 = a.copy();
        GCounter ab = a.copy();
        ab.merge(b);
        abc1 = ab.copy();
        abc1.merge(c);

        // a merge (b merge c)
        GCounter bc = b.copy();
        bc.merge(c);
        GCounter abc2 = a.copy();
        abc2.merge(bc);

        assertThat(abc1.value()).isEqualTo(abc2.value());
    }

    @Test
    void twoNodesConverge_afterMutualMerge() {
        GCounter nodeA = new GCounter();
        GCounter nodeB = new GCounter();

        // Independent increments on each node.
        for (int i = 0; i < 5; i++) nodeA.increment("java-nodeA");
        for (int i = 0; i < 3; i++) nodeB.increment("java-nodeB");

        // Exchange full state (simulate gossip sync).
        nodeA.merge(nodeB);
        nodeB.merge(nodeA);

        // Both should now agree on the same value.
        assertThat(nodeA.value()).isEqualTo(8L);
        assertThat(nodeB.value()).isEqualTo(8L);
    }
}
