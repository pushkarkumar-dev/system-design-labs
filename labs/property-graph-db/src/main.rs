//! Property Graph Database — demo: social network graph + Cypher-lite queries.

use std::collections::HashMap;
use property_graph_db::{
    executor::execute,
    graph::Graph,
    index::{LabelIndex, PropertyIndex},
    path::{page_rank, shortest_path},
    query::parse,
    NodeId,
    Value,
};

fn main() {
    println!("=== Property Graph Database Demo ===\n");

    // ── Build a social network graph ──────────────────────────────────────────
    let mut g = Graph::new();
    let mut label_idx = LabelIndex::new();
    let mut prop_idx = PropertyIndex::new();

    let alice = g.add_node(vec!["Person".into()], props(&[("name", "Alice"), ("age", "30")]));
    let bob   = g.add_node(vec!["Person".into()], props(&[("name", "Bob"),   ("age", "25")]));
    let carol = g.add_node(vec!["Person".into()], props(&[("name", "Carol"), ("age", "28")]));
    let dave  = g.add_node(vec!["Person".into()], props(&[("name", "Dave"),  ("age", "35")]));
    let eve   = g.add_node(vec!["Person".into()], props(&[("name", "Eve"),   ("age", "22")]));
    let matrix = g.add_node(vec!["Movie".into()], {
        let mut m = HashMap::new();
        m.insert("title".into(), Value::String("The Matrix".into()));
        m
    });

    // Index all nodes
    for id in [alice, bob, carol, dave, eve, matrix] {
        label_idx.insert(g.get_node(id).unwrap());
    }

    // Build property index on :Person(name)
    prop_idx.create_index("Person", "name", g.nodes.values());

    // Relationships
    g.add_edge(alice, bob,   "KNOWS".into(), HashMap::new());
    g.add_edge(bob,   carol, "KNOWS".into(), HashMap::new());
    g.add_edge(carol, dave,  "KNOWS".into(), HashMap::new());
    g.add_edge(dave,  eve,   "KNOWS".into(), HashMap::new());
    g.add_edge(alice, carol, "KNOWS".into(), HashMap::new());
    g.add_edge(alice, matrix, "WATCHED".into(), {
        let mut m = HashMap::new();
        m.insert("rating".into(), Value::Int(5));
        m
    });

    println!("Graph: {} nodes, {} edges", g.node_count(), g.edge_count());
    println!();

    // ── v0: basic queries ─────────────────────────────────────────────────────
    println!("--- v0: adjacency list ---");
    let mut nbrs = g.neighbors(alice);
    nbrs.sort();
    println!("Alice's neighbors: {:?}", nbrs);
    println!("Linear scan for :Person nodes: {}", g.find_nodes("Person").len());
    println!();

    // ── v1: Cypher-lite queries ───────────────────────────────────────────────
    println!("--- v1: Cypher-lite queries ---");

    let q1 = parse("MATCH (n:Person) RETURN n").unwrap();
    let r1 = execute(&q1, &g);
    println!("MATCH (n:Person) RETURN n => {} rows", r1.len());

    let q2 = parse(r#"MATCH (n:Person {name: "Alice"}) RETURN n"#).unwrap();
    let r2 = execute(&q2, &g);
    println!(r#"MATCH (n:Person {{name: "Alice"}}) RETURN n => {} row"#, r2.len());

    let q3 = parse("MATCH (n:Person)-[r:KNOWS]->(m:Person) RETURN n, m").unwrap();
    let r3 = execute(&q3, &g);
    println!("MATCH (n:Person)-[r:KNOWS]->(m:Person) RETURN n, m => {} rows", r3.len());

    let q4 = parse(r#"MATCH (n:Person)-[:KNOWS*1..3]->(m:Person) WHERE n.name = "Alice" RETURN m"#).unwrap();
    let r4 = execute(&q4, &g);
    println!("MATCH (n:Person)-[:KNOWS*1..3]->(m) WHERE n.name = Alice => {} rows", r4.len());
    for row in &r4 {
        if let Some(node) = row.get("m") {
            println!("  found: {:?}", node.properties.get("name"));
        }
    }
    println!();

    // ── v2: index lookup ─────────────────────────────────────────────────────
    println!("--- v2: index lookups ---");

    // LabelIndex O(1) lookup
    let person_ids = label_idx.get("Person").map(|s| s.len()).unwrap_or(0);
    println!("LabelIndex lookup :Person => {} nodes (O(1))", person_ids);

    // PropertyIndex lookup
    let result = prop_idx.lookup("Person", "name", &Value::String("Carol".into()));
    println!("PropertyIndex :Person(name='Carol') => {:?}", result);

    // BFS shortest path: Alice -> Eve
    let path = shortest_path(&g, alice, eve, None);
    println!("BFS Alice -> Eve: {:?}", path.as_ref().map(|p| p.len()));

    // PageRank
    let ranks = page_rank(&g, 20, 0.85);
    let total: f64 = ranks.values().sum();
    println!("PageRank total = {:.4} (should be ~1.0)", total);
    let mut ranked: Vec<(NodeId, f64)> = ranks.into_iter().collect();
    ranked.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap());
    println!("Top-ranked nodes (by PageRank):");
    for (id, score) in &ranked[..ranked.len().min(3)] {
        let name = g.get_node(*id)
            .and_then(|n| n.properties.get("name"))
            .map(|v| v.to_string())
            .unwrap_or_else(|| format!("id={}", id));
        println!("  {} => {:.4}", name, score);
    }
}

fn props(pairs: &[(&str, &str)]) -> HashMap<String, Value> {
    pairs
        .iter()
        .map(|(k, v)| {
            let val = if let Ok(i) = v.parse::<i64>() {
                Value::Int(i)
            } else {
                Value::String(v.to_string())
            };
            (k.to_string(), val)
        })
        .collect()
}
