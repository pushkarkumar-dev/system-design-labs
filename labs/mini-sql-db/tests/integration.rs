// integration.rs — end-to-end SQL tests using the Database API
//
// Each test exercises a full SQL path: tokenize -> parse -> plan -> execute.
// These tests are the acceptance criteria for each lab stage.

use mini_sql_db::db::Database;
use mini_sql_db::Value;

fn db() -> Database { Database::new() }

// ── v0 tests: CREATE / INSERT / SELECT ───────────────────────────────────────

#[test]
fn create_table_creates_schema() {
    let mut db = db();
    db.execute("CREATE TABLE products (id INT, name TEXT, price INT)").unwrap();
    assert!(db.tables.contains_key("products"));
    assert_eq!(db.tables["products"].schema.len(), 3);
}

#[test]
fn insert_and_select_star() {
    let mut db = db();
    db.execute("CREATE TABLE t (id INT, label TEXT)").unwrap();
    db.execute("INSERT INTO t (id, label) VALUES (1, 'alpha'), (2, 'beta')").unwrap();
    let r = db.execute("SELECT * FROM t").unwrap();
    assert_eq!(r.rows.len(), 2);
}

#[test]
fn select_with_eq_where() {
    let mut db = db();
    db.execute("CREATE TABLE employees (id INT, dept TEXT, salary INT)").unwrap();
    db.execute("INSERT INTO employees (id, dept, salary) VALUES (1, 'eng', 120), (2, 'mkt', 90), (3, 'eng', 110)").unwrap();
    let r = db.execute("SELECT id, salary FROM employees WHERE dept = 'eng'").unwrap();
    assert_eq!(r.rows.len(), 2);
    assert!(r.rows.iter().all(|row| row.get("dept").is_none())); // projection
}

#[test]
fn select_with_gt_where() {
    let mut db = db();
    db.execute("CREATE TABLE scores (player TEXT, score INT)").unwrap();
    for (p, s) in [("a", 10), ("b", 50), ("c", 30), ("d", 70)] {
        db.execute(&format!("INSERT INTO scores (player, score) VALUES ('{}', {})", p, s)).unwrap();
    }
    let r = db.execute("SELECT player FROM scores WHERE score > 40").unwrap();
    assert_eq!(r.rows.len(), 2);
}

#[test]
fn type_coercion_text_to_int() {
    let mut db = db();
    db.execute("CREATE TABLE t (n INT)").unwrap();
    db.execute("INSERT INTO t (n) VALUES ('100')").unwrap();
    let r = db.execute("SELECT * FROM t").unwrap();
    assert_eq!(r.rows[0]["n"], Value::Int(100));
}

#[test]
fn unknown_column_error() {
    let mut db = db();
    db.execute("CREATE TABLE t (id INT)").unwrap();
    let err = db.execute("SELECT ghost FROM t").unwrap_err();
    assert!(matches!(err, mini_sql_db::SqlError::ColumnNotFound(_)));
}

#[test]
fn missing_table_error() {
    let mut db = db();
    let err = db.execute("SELECT * FROM phantom").unwrap_err();
    assert!(matches!(err, mini_sql_db::SqlError::TableNotFound(_)));
}

#[test]
fn multi_condition_where_and() {
    let mut db = db();
    db.execute("CREATE TABLE t (a INT, b INT)").unwrap();
    db.execute("INSERT INTO t (a, b) VALUES (1, 10), (2, 20), (1, 30)").unwrap();
    let r = db.execute("SELECT b FROM t WHERE a = 1 AND b > 15").unwrap();
    assert_eq!(r.rows.len(), 1);
    assert_eq!(r.rows[0]["b"], Value::Int(30));
}

// ── v1 tests: JOIN / ORDER BY / LIMIT / OFFSET ───────────────────────────────

#[test]
fn join_two_tables() {
    let mut db = db();
    db.execute("CREATE TABLE users (uid INT, username TEXT)").unwrap();
    db.execute("CREATE TABLE orders (oid INT, uid INT, total INT)").unwrap();
    db.execute("INSERT INTO users (uid, username) VALUES (1, 'alice'), (2, 'bob')").unwrap();
    db.execute("INSERT INTO orders (oid, uid, total) VALUES (100, 1, 50), (101, 2, 75), (102, 1, 25)").unwrap();
    let r = db.execute("SELECT uid, total FROM users INNER JOIN orders ON users.uid = orders.uid").unwrap();
    assert_eq!(r.rows.len(), 3); // alice has 2 orders, bob has 1
}

#[test]
fn order_by_descending() {
    let mut db = db();
    db.execute("CREATE TABLE t (v INT)").unwrap();
    for v in [5, 2, 8, 1, 9, 3] {
        db.execute(&format!("INSERT INTO t (v) VALUES ({})", v)).unwrap();
    }
    let r = db.execute("SELECT v FROM t ORDER BY v DESC").unwrap();
    assert_eq!(r.rows[0]["v"], Value::Int(9));
    assert_eq!(r.rows[1]["v"], Value::Int(8));
    assert_eq!(r.rows[2]["v"], Value::Int(5));
}

#[test]
fn limit_returns_correct_count() {
    let mut db = db();
    db.execute("CREATE TABLE t (n INT)").unwrap();
    for i in 1..=20 {
        db.execute(&format!("INSERT INTO t (n) VALUES ({})", i)).unwrap();
    }
    let r = db.execute("SELECT n FROM t LIMIT 5").unwrap();
    assert_eq!(r.rows.len(), 5);
}

#[test]
fn offset_skips_rows() {
    let mut db = db();
    db.execute("CREATE TABLE t (n INT)").unwrap();
    for i in 1..=10 {
        db.execute(&format!("INSERT INTO t (n) VALUES ({})", i)).unwrap();
    }
    // ORDER BY n ASC OFFSET 3 LIMIT 2 -> rows 4 and 5
    let r = db.execute("SELECT n FROM t ORDER BY n ASC LIMIT 2 OFFSET 3").unwrap();
    assert_eq!(r.rows.len(), 2);
    assert_eq!(r.rows[0]["n"], Value::Int(4));
    assert_eq!(r.rows[1]["n"], Value::Int(5));
}

#[test]
fn projection_narrows_columns() {
    let mut db = db();
    db.execute("CREATE TABLE wide (a INT, b TEXT, c INT, d TEXT)").unwrap();
    db.execute("INSERT INTO wide (a, b, c, d) VALUES (1, 'x', 2, 'y')").unwrap();
    let r = db.execute("SELECT a, c FROM wide").unwrap();
    assert_eq!(r.rows.len(), 1);
    assert!(r.rows[0].contains_key("a"));
    assert!(r.rows[0].contains_key("c"));
    assert!(!r.rows[0].contains_key("b"));
    assert!(!r.rows[0].contains_key("d"));
}

// ── v2 tests: index / transactions ───────────────────────────────────────────

#[test]
fn create_index_and_lookup() {
    let mut db = db();
    db.execute("CREATE TABLE items (id INT, name TEXT)").unwrap();
    for i in 1..=50i64 {
        db.execute(&format!("INSERT INTO items (id, name) VALUES ({}, 'item{}')", i, i)).unwrap();
    }
    db.execute("CREATE INDEX idx ON items (id)").unwrap();
    assert!(db.indexes.has_index("items", "id"));
}

#[test]
fn index_scan_returns_correct_row() {
    let mut db = db();
    db.execute("CREATE TABLE products (id INT, price INT)").unwrap();
    for i in 1..=200i64 {
        db.execute(&format!("INSERT INTO products (id, price) VALUES ({}, {})", i, i * 5)).unwrap();
    }
    db.execute("CREATE INDEX idx_pid ON products (id)").unwrap();
    let r = db.execute("SELECT price FROM products WHERE id = 150").unwrap();
    assert_eq!(r.rows.len(), 1);
    assert_eq!(r.rows[0]["price"], Value::Int(750));
}

#[test]
fn transaction_commit_persists() {
    let mut db = db();
    db.execute("CREATE TABLE accounts (id INT, balance INT)").unwrap();
    db.execute("BEGIN").unwrap();
    db.execute("INSERT INTO accounts (id, balance) VALUES (1, 1000)").unwrap();
    db.execute("INSERT INTO accounts (id, balance) VALUES (2, 2000)").unwrap();
    db.execute("COMMIT").unwrap();
    let r = db.execute("SELECT * FROM accounts").unwrap();
    assert_eq!(r.rows.len(), 2);
}

#[test]
fn transaction_rollback_discards_rows() {
    let mut db = db();
    db.execute("CREATE TABLE log (event TEXT)").unwrap();
    // Committed row
    db.execute("INSERT INTO log (event) VALUES ('baseline')").unwrap();
    // Start tx, insert, rollback
    db.execute("BEGIN").unwrap();
    db.execute("INSERT INTO log (event) VALUES ('uncommitted-a')").unwrap();
    db.execute("INSERT INTO log (event) VALUES ('uncommitted-b')").unwrap();
    db.execute("ROLLBACK").unwrap();
    let r = db.execute("SELECT * FROM log").unwrap();
    assert_eq!(r.rows.len(), 1, "only the committed baseline row should remain");
    assert_eq!(r.rows[0]["event"], Value::Text("baseline".into()));
}

#[test]
fn wal_entry_count_reflects_operations() {
    let mut db = db();
    db.execute("CREATE TABLE t (x INT)").unwrap(); // begin + create + commit = 3
    db.execute("INSERT INTO t (x) VALUES (1)").unwrap(); // begin + insert + commit = 3
    // Total: 6 entries minimum
    assert!(db.wal.len() >= 6);
}
