//! v1 — Query executor: runs QueryPlan against a Graph.
//!
//! Supported operations:
//!   - ScanNodes: full label scan (O(n))
//!   - Filter: apply Predicate to reduce result set
//!   - PathMatch: traverse node-rel-node patterns, with variable-length DFS

use std::collections::HashMap;

use crate::{
    graph::Graph,
    query::{Predicate, QueryPlan},
    Node, NodeId, Value,
};

/// A row in the result set: maps alias -> node reference.
/// For simplicity we clone nodes into the result (no lifetime gymnastics).
pub type ResultRow = HashMap<String, Node>;

/// Execute a query plan against the graph and return result rows.
pub fn execute(plan: &QueryPlan, graph: &Graph) -> Vec<ResultRow> {
    match plan {
        QueryPlan::ScanNodes(label) => scan_nodes(label, graph),

        QueryPlan::Filter(inner, pred) => {
            let rows = execute(inner, graph);
            apply_filter(rows, pred)
        }

        QueryPlan::Project(inner, columns) => {
            let rows = execute(inner, graph);
            project_rows(rows, columns)
        }

        QueryPlan::PathMatch {
            from_label,
            from_props,
            rel_type,
            min_hops,
            max_hops,
            to_label,
            where_pred,
            return_aliases,
            from_alias,
            to_alias,
        } => {
            let results = match_path(
                from_label.as_deref(),
                from_props,
                rel_type.as_deref(),
                *min_hops,
                *max_hops,
                to_label.as_deref(),
                where_pred.as_ref(),
                graph,
                from_alias,
                to_alias,
            );
            project_rows(results, return_aliases)
        }

        QueryPlan::Traverse { source, rel_type, min_hops, max_hops } => {
            // Generic traverse: for each node in source, DFS up to max_hops
            let source_rows = execute(source, graph);
            let mut results = Vec::new();
            for row in &source_rows {
                if let Some((alias, src_node)) = row.iter().next() {
                    let reachable = dfs_traverse(graph, src_node.id, rel_type.as_deref(), *min_hops, *max_hops);
                    for nid in reachable {
                        if let Some(n) = graph.get_node(nid) {
                            let mut r = HashMap::new();
                            r.insert(alias.clone(), src_node.clone());
                            r.insert("_target".to_string(), n.clone());
                            results.push(r);
                        }
                    }
                }
            }
            results
        }
    }
}

// ── Internal helpers ──────────────────────────────────────────────────────────

fn scan_nodes(label: &str, graph: &Graph) -> Vec<ResultRow> {
    graph
        .find_nodes(label)
        .into_iter()
        .map(|n| {
            let mut row = HashMap::new();
            row.insert("n".to_string(), n.clone());
            row
        })
        .collect()
}

fn apply_filter(rows: Vec<ResultRow>, pred: &Predicate) -> Vec<ResultRow> {
    rows.into_iter()
        .filter(|row| eval_predicate(row, pred))
        .collect()
}

fn eval_predicate(row: &ResultRow, pred: &Predicate) -> bool {
    match pred {
        Predicate::Eq(alias, prop, expected) => {
            if let Some(node) = row.get(alias) {
                node.properties.get(prop) == Some(expected)
            } else {
                false
            }
        }
    }
}

fn project_rows(rows: Vec<ResultRow>, columns: &[String]) -> Vec<ResultRow> {
    rows.into_iter()
        .map(|mut row| {
            let mut projected = HashMap::new();
            for col in columns {
                if let Some(v) = row.remove(col) {
                    projected.insert(col.clone(), v);
                }
            }
            projected
        })
        .collect()
}

/// Match a path pattern: (from:label {props})-[:type*min..max]->(to:label)
fn match_path(
    from_label: Option<&str>,
    from_props: &HashMap<String, Value>,
    rel_type: Option<&str>,
    min_hops: usize,
    max_hops: usize,
    to_label: Option<&str>,
    where_pred: Option<&Predicate>,
    graph: &Graph,
    from_alias: &str,
    to_alias: &str,
) -> Vec<ResultRow> {
    let mut results = Vec::new();

    // Collect candidate "from" nodes
    let from_nodes: Vec<&Node> = if let Some(label) = from_label {
        graph.find_nodes(label)
    } else {
        graph.nodes.values().collect()
    };

    for from_node in from_nodes {
        // Apply inline property filter on from-node
        if !props_match(from_node, from_props) {
            continue;
        }

        // DFS from this node
        let reachable = dfs_traverse(graph, from_node.id, rel_type, min_hops, max_hops);

        for target_id in reachable {
            let to_node = match graph.get_node(target_id) {
                Some(n) => n,
                None => continue,
            };

            // Apply to-label filter
            if let Some(label) = to_label {
                if !to_node.labels.iter().any(|l| l == label) {
                    continue;
                }
            }

            // Build a candidate row for WHERE evaluation
            let mut row: ResultRow = HashMap::new();
            row.insert(from_alias.to_string(), from_node.clone());
            row.insert(to_alias.to_string(), to_node.clone());

            // Apply WHERE predicate
            if let Some(pred) = where_pred {
                if !eval_predicate(&row, pred) {
                    continue;
                }
            }

            results.push(row);
        }
    }

    results
}

fn props_match(node: &Node, required: &HashMap<String, Value>) -> bool {
    required.iter().all(|(k, v)| node.properties.get(k) == Some(v))
}

/// DFS traversal: collect NodeIds reachable from `start` via edges with
/// the given `rel_type` (None = any type), between `min_hops` and `max_hops`.
pub fn dfs_traverse(
    graph: &Graph,
    start: NodeId,
    rel_type: Option<&str>,
    min_hops: usize,
    max_hops: usize,
) -> Vec<NodeId> {
    let mut results = Vec::new();
    let mut stack: Vec<(NodeId, usize)> = vec![(start, 0)];
    let mut visited_at_depth: Vec<(NodeId, usize)> = Vec::new();

    while let Some((current, depth)) = stack.pop() {
        if depth > max_hops {
            continue;
        }
        if visited_at_depth.contains(&(current, depth)) {
            continue;
        }
        visited_at_depth.push((current, depth));

        if depth >= min_hops && current != start {
            results.push(current);
        }

        if depth < max_hops {
            for edge in graph.outgoing_edges(current) {
                if let Some(rt) = rel_type {
                    if edge.label != rt {
                        continue;
                    }
                }
                stack.push((edge.to, depth + 1));
            }
        }
    }

    results
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::graph::Graph;
    use crate::query::parse;

    fn build_social_graph() -> Graph {
        let mut g = Graph::new();
        let alice = g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Alice".into()));
            m
        });
        let bob = g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Bob".into()));
            m
        });
        let carol = g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Carol".into()));
            m
        });
        let dave = g.add_node(vec!["Person".into()], {
            let mut m = HashMap::new();
            m.insert("name".into(), Value::String("Dave".into()));
            m
        });
        g.add_node(vec!["Movie".into()], {
            let mut m = HashMap::new();
            m.insert("title".into(), Value::String("The Matrix".into()));
            m
        });
        // Alice -> Bob -> Carol -> Dave (chain)
        g.add_edge(alice, bob, "KNOWS".into(), HashMap::new());
        g.add_edge(bob, carol, "KNOWS".into(), HashMap::new());
        g.add_edge(carol, dave, "KNOWS".into(), HashMap::new());
        g
    }

    #[test]
    fn test_match_all_nodes_of_label() {
        let g = build_social_graph();
        let plan = parse("MATCH (n:Person) RETURN n").unwrap();
        let rows = execute(&plan, &g);
        assert_eq!(rows.len(), 4);
        for row in &rows {
            let node = row.get("n").expect("alias 'n' in row");
            assert!(node.labels.contains(&"Person".to_string()));
        }
    }

    #[test]
    fn test_match_with_property_filter() {
        let g = build_social_graph();
        let plan = parse(r#"MATCH (n:Person {name: "Alice"}) RETURN n"#).unwrap();
        let rows = execute(&plan, &g);
        assert_eq!(rows.len(), 1);
        let node = rows[0].get("n").unwrap();
        assert_eq!(node.properties.get("name"), Some(&Value::String("Alice".into())));
    }

    #[test]
    fn test_match_simple_relationship() {
        let g = build_social_graph();
        let plan = parse("MATCH (n:Person)-[r:KNOWS]->(m:Person) RETURN n, m").unwrap();
        let rows = execute(&plan, &g);
        // Alice->Bob, Bob->Carol, Carol->Dave = 3 relationships
        assert_eq!(rows.len(), 3);
        for row in &rows {
            assert!(row.contains_key("n"));
            assert!(row.contains_key("m"));
        }
    }

    #[test]
    fn test_variable_length_path_finds_3_hop_friends() {
        let g = build_social_graph();
        // Alice -KNOWS*1..3-> should reach Bob (1), Carol (2), Dave (3)
        let plan = parse(r#"MATCH (n:Person)-[:KNOWS*1..3]->(m:Person) WHERE n.name = "Alice" RETURN m"#).unwrap();
        let rows = execute(&plan, &g);
        // Bob, Carol, Dave
        assert_eq!(rows.len(), 3);
    }

    #[test]
    fn test_where_clause_filters_results() {
        let g = build_social_graph();
        let plan = parse(r#"MATCH (n:Person) WHERE n.name = "Bob" RETURN n"#).unwrap();
        let rows = execute(&plan, &g);
        assert_eq!(rows.len(), 1);
        let node = rows[0].get("n").unwrap();
        assert_eq!(node.properties.get("name"), Some(&Value::String("Bob".into())));
    }

    #[test]
    fn test_return_projects_correct_columns() {
        let g = build_social_graph();
        let plan = parse("MATCH (n:Person)-[r:KNOWS]->(m:Person) RETURN n, m").unwrap();
        let rows = execute(&plan, &g);
        assert!(!rows.is_empty());
        // Every row must have exactly 'n' and 'm'
        for row in &rows {
            assert!(row.contains_key("n"), "row missing 'n'");
            assert!(row.contains_key("m"), "row missing 'm'");
            assert_eq!(row.len(), 2, "row should have exactly 2 columns");
        }
    }
}
