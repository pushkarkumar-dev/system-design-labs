//! Demo: walk through v0 → v1 → v2, showing the key behaviors at each stage.
//!
//! Run: cargo run --example demo

use serde_json::json;
use tempfile::TempDir;

use document_db::v0::DocumentStore as StoreV0;
use document_db::v1::DocumentStore as StoreV1;
use document_db::v2::DocumentStore as StoreV2;
use document_db::Filter;

fn main() {
    demo_v0();
    demo_v1();
    demo_v2();
}

fn demo_v0() {
    println!("=== v0 — in-memory document store ===");
    let mut store = StoreV0::new();

    // Insert documents with different shapes — no schema
    let id1 = store.insert("users", json!({"name": "Alice", "email": "alice@example.com", "age": 30}));
    let id2 = store.insert("users", json!({"name": "Bob", "email": "bob@example.com", "age": 25, "premium": true}));
    let _id3 = store.insert("users", json!({"name": "Carol", "email": "carol@example.com"}));

    println!("Inserted 3 users with IDs:");
    println!("  Alice: {id1}");
    println!("  Bob:   {id2}");

    // Get by ID
    if let Some(alice) = store.get("users", &id1) {
        println!("\nFetched Alice: {}", alice["name"]);
    }

    // Find by filter
    let mut filter = Filter::new();
    filter.insert("age".into(), json!(30));
    let results = store.find("users", &filter);
    println!("\nFind {{age: 30}}: {} result(s)", results.len());

    // Empty filter = all docs
    let all = store.find("users", &Filter::new());
    println!("Find {{}} (all): {} results", all.len());

    // Different collection shapes in the same store
    store.insert("events", json!({"type": "click", "element": "button#cta"}));
    store.insert("events", json!({"type": "purchase", "amount": 49.99, "items": [1, 2, 3]}));
    println!("\nEvents collection: {} docs (mixed shapes)", store.count("events"));
    println!();
}

fn demo_v1() {
    println!("=== v1 — BSON-like binary encoding on disk ===");
    let dir = TempDir::new().unwrap();

    let id = {
        let mut store = StoreV1::open(dir.path()).unwrap();
        let id = store.insert("products", json!({
            "name": "Laptop",
            "price": 999.99_f64,
            "stock": 42_i64,
            "available": true,
            "tags": ["electronics", "computers"]
        })).unwrap();
        store.insert("products", json!({"name": "Mouse", "price": 29.99_f64, "stock": 150_i64, "available": true})).unwrap();
        store.insert("products", json!({"name": "Desk", "price": 349.99_f64, "stock": 5_i64, "available": false})).unwrap();
        println!("Inserted 3 products to disk");
        id
    };

    // Reopen — documents survive
    let store = StoreV1::open(dir.path()).unwrap();
    let laptop = store.get("products", &id).unwrap().expect("laptop not found after reopen");
    println!("After reopen, fetched: {} (price: {})", laptop["name"], laptop["price"]);

    // Find available products
    let mut f = Filter::new();
    f.insert("available".into(), json!(true));
    let available = store.find("products", &f).unwrap();
    println!("Available products: {}", available.len());
    println!();
}

fn demo_v2() {
    println!("=== v2 — secondary indexes ===");
    let dir = TempDir::new().unwrap();
    let mut store = StoreV2::open(dir.path()).unwrap();

    // Pre-create index on email before inserts
    store.create_index("users", "email").unwrap();

    // Insert 1000 users
    for i in 0..1000 {
        store.insert("users", json!({
            "email": format!("user{}@example.com", i),
            "role": if i % 5 == 0 { "admin" } else { "user" },
            "age": 20 + (i % 40)
        })).unwrap();
    }
    println!("Inserted 1000 users");

    // Indexed find by email
    let mut f = Filter::new();
    f.insert("email".into(), json!("user42@example.com"));
    let found = store.find("users", &f).unwrap();
    println!("Indexed find {{email: user42@example.com}}: {} result(s)", found.len());
    if let Some(u) = found.first() {
        println!("  Found: email={}, role={}", u["email"], u["role"]);
    }

    // Create index on role AFTER inserts
    store.create_index("users", "role").unwrap();
    let mut f2 = Filter::new();
    f2.insert("role".into(), json!("admin"));
    let admins = store.find("users", &f2).unwrap();
    println!("Indexed find {{role: admin}}: {} result(s) (expected 200)", admins.len());

    // Demonstrate index selectivity with boolean
    store.create_index("users", "role").unwrap();
    println!("\nIndex on 'role' (5 distinct values): moderate selectivity");
    println!("Index on 'email' (1000 distinct values): high selectivity");
    println!("Index on boolean (2 values): low selectivity — avoid");
    println!();
}
