use criterion::{black_box, criterion_group, criterion_main, BenchmarkId, Criterion, Throughput};
use property_graph_db::{
    graph::Graph,
    index::LabelIndex,
    path::{bfs_shortest_path, page_rank},
    Value,
};
use std::collections::HashMap;

fn bench_add_node(c: &mut Criterion) {
    let mut group = c.benchmark_group("add-node");
    group.throughput(Throughput::Elements(1));

    group.bench_function("add-node-no-props", |b| {
        let mut g = Graph::new();
        b.iter(|| {
            g.add_node(vec!["Person".into()], HashMap::new());
        });
    });

    group.bench_function("add-node-with-props", |b| {
        let mut g = Graph::new();
        b.iter(|| {
            let mut props = HashMap::new();
            props.insert("name".to_string(), Value::String(black_box("Alice".to_string())));
            props.insert("age".to_string(), Value::Int(black_box(30)));
            g.add_node(vec!["Person".into()], props);
        });
    });

    group.finish();
}

fn bench_find_nodes_linear(c: &mut Criterion) {
    let mut group = c.benchmark_group("find-nodes");
    group.throughput(Throughput::Elements(1));

    for size in [1_000usize, 10_000, 100_000] {
        let mut g = Graph::new();
        for i in 0..size {
            let mut props = HashMap::new();
            props.insert("name".to_string(), Value::String(format!("Person{}", i)));
            g.add_node(vec!["Person".into()], props);
        }

        group.bench_with_input(
            BenchmarkId::new("linear-scan", size),
            &size,
            |b, _| {
                b.iter(|| {
                    let _ = black_box(g.find_nodes("Person").len());
                });
            },
        );
    }

    group.finish();
}

fn bench_find_nodes_indexed(c: &mut Criterion) {
    let mut group = c.benchmark_group("find-nodes-indexed");
    group.throughput(Throughput::Elements(1));

    for size in [1_000usize, 10_000, 100_000] {
        let mut g = Graph::new();
        let mut idx = LabelIndex::new();
        for i in 0..size {
            let mut props = HashMap::new();
            props.insert("name".to_string(), Value::String(format!("Person{}", i)));
            let id = g.add_node(vec!["Person".into()], props);
            idx.insert(g.get_node(id).unwrap());
        }

        group.bench_with_input(
            BenchmarkId::new("label-index", size),
            &size,
            |b, _| {
                b.iter(|| {
                    let _ = black_box(idx.get("Person").map(|s| s.len()));
                });
            },
        );
    }

    group.finish();
}

fn bench_bfs(c: &mut Criterion) {
    let mut group = c.benchmark_group("bfs-shortest-path");

    // Build a chain of nodes
    let mut g = Graph::new();
    let mut ids: Vec<u64> = Vec::new();
    for i in 0..10_000usize {
        let mut props = HashMap::new();
        props.insert("i".to_string(), Value::Int(i as i64));
        let id = g.add_node(vec!["Node".into()], props);
        ids.push(id);
    }
    for i in 0..ids.len() - 1 {
        g.add_edge(ids[i], ids[i + 1], "NEXT".into(), HashMap::new());
    }

    group.bench_function("bfs-100-hops", |b| {
        b.iter(|| {
            let _ = black_box(bfs_shortest_path(&g, ids[0], ids[100]));
        });
    });

    group.finish();
}

fn bench_pagerank(c: &mut Criterion) {
    let mut group = c.benchmark_group("pagerank");

    let mut g = Graph::new();
    let mut ids: Vec<u64> = Vec::new();
    for i in 0..1_000usize {
        let id = g.add_node(vec!["Node".into()], HashMap::new());
        ids.push(id);
    }
    // Ring topology to avoid dangling nodes
    for i in 0..ids.len() {
        g.add_edge(ids[i], ids[(i + 1) % ids.len()], "LINK".into(), HashMap::new());
    }

    group.bench_function("pagerank-50-iters-1k-nodes", |b| {
        b.iter(|| {
            let _ = black_box(page_rank(&g, 50, 0.85));
        });
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_add_node,
    bench_find_nodes_linear,
    bench_find_nodes_indexed,
    bench_bfs,
    bench_pagerank
);
criterion_main!(benches);
