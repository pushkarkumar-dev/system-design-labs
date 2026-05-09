// main.rs — mini-sql-db REPL
//
// Reads SQL from stdin line by line, executes against an in-memory Database,
// and prints a formatted result table.  No network server — pure I/O loop.
//
// Example:
//   cargo run --bin mini-sql-db
//   > CREATE TABLE users (id INT, name TEXT);
//   > INSERT INTO users (id, name) VALUES (1, 'Alice'), (2, 'Bob');
//   > SELECT * FROM users WHERE id = 1;

use mini_sql_db::db::Database;

fn main() {
    let mut db = Database::new();
    let stdin = std::io::stdin();
    let mut input = String::new();

    eprintln!("mini-sql-db v0.1 — type SQL and press Enter.  Ctrl-D to exit.");
    eprint!("> ");

    loop {
        input.clear();
        match stdin.read_line(&mut input) {
            Ok(0) => break, // EOF
            Ok(_) => {}
            Err(e) => { eprintln!("read error: {}", e); break; }
        }

        let sql = input.trim();
        if sql.is_empty() {
            eprint!("> ");
            continue;
        }

        match db.execute(sql) {
            Ok(result) => {
                if !result.columns.is_empty() {
                    print_table(&result.columns, &result.rows);
                } else {
                    println!("-- {}", result.message);
                }
            }
            Err(e) => eprintln!("ERROR: {}", e),
        }

        eprint!("> ");
    }

    eprintln!("Bye.");
}

fn print_table(columns: &[String], rows: &[mini_sql_db::Row]) {
    // Calculate column widths
    let mut widths: Vec<usize> = columns.iter().map(|c| c.len()).collect();
    for row in rows {
        for (i, col) in columns.iter().enumerate() {
            let val_len = row.get(col).map(|v| v.to_string().len()).unwrap_or(4);
            widths[i] = widths[i].max(val_len);
        }
    }

    // Header
    let header: Vec<String> = columns.iter().enumerate()
        .map(|(i, col)| format!("{:<width$}", col, width = widths[i]))
        .collect();
    println!("| {} |", header.join(" | "));

    // Separator
    let sep: Vec<String> = widths.iter().map(|&w| "-".repeat(w)).collect();
    println!("+-{}-+", sep.join("-+-"));

    // Rows
    for row in rows {
        let cells: Vec<String> = columns.iter().enumerate()
            .map(|(i, col)| {
                let val = row.get(col).map(|v| v.to_string()).unwrap_or_else(|| "NULL".into());
                format!("{:<width$}", val, width = widths[i])
            })
            .collect();
        println!("| {} |", cells.join(" | "));
    }

    println!("({} row{})", rows.len(), if rows.len() == 1 { "" } else { "s" });
}

use std::io::BufRead;
