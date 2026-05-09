// wal.rs — v2 in-memory Write-Ahead Log
//
// Simplified WAL: entries are accumulated in a Vec rather than written to
// disk.  In a production database, each entry would be fsynced to a WAL file
// before the data page is modified.
//
// Entry types we log:
//   - InsertRow: logged before each INSERT so we can replay on restart
//   - CreateTable: logged before table creation
//   - BeginTx / CommitTx / RollbackTx: transaction boundaries
//
// Replay: on startup, replay all committed transactions to rebuild state.

use std::collections::HashMap;

/// A single WAL entry.
#[derive(Debug, Clone)]
pub enum LogEntry {
    BeginTx    { tx_id: u64 },
    CommitTx   { tx_id: u64 },
    RollbackTx { tx_id: u64 },
    CreateTable { tx_id: u64, table_name: String, schema: Vec<(String, String)> },
    InsertRow   { tx_id: u64, table_name: String, row: HashMap<String, String> },
}

/// In-memory WAL.
pub struct Wal {
    pub entries: Vec<LogEntry>,
    pub next_tx_id: u64,
}

impl Wal {
    pub fn new() -> Self {
        Wal { entries: Vec::new(), next_tx_id: 1 }
    }

    /// Begin a new transaction and return its ID.
    pub fn begin_tx(&mut self) -> u64 {
        let tx_id = self.next_tx_id;
        self.next_tx_id += 1;
        self.entries.push(LogEntry::BeginTx { tx_id });
        tx_id
    }

    /// Record a COMMIT.
    pub fn commit_tx(&mut self, tx_id: u64) {
        self.entries.push(LogEntry::CommitTx { tx_id });
    }

    /// Record a ROLLBACK.
    pub fn rollback_tx(&mut self, tx_id: u64) {
        self.entries.push(LogEntry::RollbackTx { tx_id });
    }

    /// Log a CREATE TABLE before executing it.
    pub fn log_create_table(
        &mut self,
        tx_id: u64,
        table_name: &str,
        schema: Vec<(String, String)>,
    ) {
        self.entries.push(LogEntry::CreateTable {
            tx_id,
            table_name: table_name.to_string(),
            schema,
        });
    }

    /// Log an INSERT row.  Values are serialized to strings for simplicity.
    pub fn log_insert(
        &mut self,
        tx_id: u64,
        table_name: &str,
        row: &HashMap<String, crate::Value>,
    ) {
        let serialized: HashMap<String, String> = row.iter()
            .map(|(k, v)| (k.clone(), v.to_string()))
            .collect();
        self.entries.push(LogEntry::InsertRow {
            tx_id,
            table_name: table_name.to_string(),
            row: serialized,
        });
    }

    /// Return the IDs of all committed transactions.
    pub fn committed_tx_ids(&self) -> Vec<u64> {
        let mut committed = Vec::new();
        for entry in &self.entries {
            if let LogEntry::CommitTx { tx_id } = entry {
                committed.push(*tx_id);
            }
        }
        committed
    }

    /// Replay entries for committed transactions.
    /// Returns (table_schemas, rows_per_table) for rebuilding state.
    pub fn replay(&self) -> ReplayResult {
        let committed = self.committed_tx_ids();
        let committed_set: std::collections::HashSet<u64> = committed.into_iter().collect();

        let mut schemas: HashMap<String, Vec<(String, String)>> = HashMap::new();
        let mut rows: HashMap<String, Vec<HashMap<String, String>>> = HashMap::new();

        for entry in &self.entries {
            match entry {
                LogEntry::CreateTable { tx_id, table_name, schema } => {
                    if committed_set.contains(tx_id) {
                        schemas.insert(table_name.clone(), schema.clone());
                    }
                }
                LogEntry::InsertRow { tx_id, table_name, row } => {
                    if committed_set.contains(tx_id) {
                        rows.entry(table_name.clone())
                            .or_default()
                            .push(row.clone());
                    }
                }
                _ => {}
            }
        }
        ReplayResult { schemas, rows }
    }

    /// Total number of WAL entries.
    pub fn len(&self) -> usize {
        self.entries.len()
    }
}

/// Output of WAL replay: everything needed to reconstruct the catalog.
pub struct ReplayResult {
    pub schemas: HashMap<String, Vec<(String, String)>>,
    pub rows: HashMap<String, Vec<HashMap<String, String>>>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::Value;

    #[test]
    fn begin_commit_logged() {
        let mut wal = Wal::new();
        let tx_id = wal.begin_tx();
        wal.commit_tx(tx_id);
        assert_eq!(wal.committed_tx_ids(), vec![tx_id]);
        assert_eq!(wal.len(), 2);
    }

    #[test]
    fn rollback_not_replayed() {
        let mut wal = Wal::new();
        let tx1 = wal.begin_tx();
        wal.log_create_table(tx1, "t", vec![("id".into(), "INT".into())]);
        wal.rollback_tx(tx1);

        let tx2 = wal.begin_tx();
        wal.log_create_table(tx2, "t2", vec![("name".into(), "TEXT".into())]);
        wal.commit_tx(tx2);

        let replay = wal.replay();
        assert!(!replay.schemas.contains_key("t"));
        assert!(replay.schemas.contains_key("t2"));
    }

    #[test]
    fn insert_rows_replayed() {
        let mut wal = Wal::new();
        let tx = wal.begin_tx();
        let mut row = HashMap::new();
        row.insert("id".into(), Value::Int(1));
        row.insert("name".into(), Value::Text("Alice".into()));
        wal.log_insert(tx, "users", &row);
        wal.commit_tx(tx);

        let replay = wal.replay();
        let replayed_rows = replay.rows.get("users").unwrap();
        assert_eq!(replayed_rows.len(), 1);
        assert_eq!(replayed_rows[0]["id"], "1");
        assert_eq!(replayed_rows[0]["name"], "Alice");
    }

    #[test]
    fn multiple_transactions() {
        let mut wal = Wal::new();
        let tx1 = wal.begin_tx();
        let tx2 = wal.begin_tx();

        let mut r1 = HashMap::new();
        r1.insert("id".into(), Value::Int(1));
        let mut r2 = HashMap::new();
        r2.insert("id".into(), Value::Int(2));

        wal.log_insert(tx1, "t", &r1);
        wal.log_insert(tx2, "t", &r2);

        wal.commit_tx(tx1);
        wal.rollback_tx(tx2); // tx2 rolled back

        let replay = wal.replay();
        let rows = replay.rows.get("t").unwrap();
        assert_eq!(rows.len(), 1); // only tx1's row
        assert_eq!(rows[0]["id"], "1");
    }
}
