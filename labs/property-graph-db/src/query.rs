//! v1 — Cypher-lite query language: parser and QueryPlan.
//!
//! Supported patterns:
//!   MATCH (n:Label) RETURN n
//!   MATCH (n:Label {prop: "value"}) RETURN n
//!   MATCH (n:Label)-[r:TYPE]->(m:Label) RETURN n, m
//!   MATCH (n:Label)-[:TYPE*min..max]->(m:Label) WHERE n.prop = "value" RETURN m

use std::collections::HashMap;
use crate::Value;

/// A predicate used in WHERE clauses and inline property filters.
#[derive(Debug, Clone, PartialEq)]
pub enum Predicate {
    /// Property equals a literal value. `(alias, prop_name, value)`
    Eq(String, String, Value),
}

/// A node pattern extracted from the MATCH clause.
#[derive(Debug, Clone)]
pub struct NodePattern {
    pub alias: String,
    pub label: Option<String>,
    pub props: HashMap<String, Value>,
}

/// A relationship pattern extracted from the MATCH clause.
#[derive(Debug, Clone)]
pub struct RelPattern {
    pub alias: Option<String>,
    pub rel_type: Option<String>,
    pub min_hops: usize,
    pub max_hops: usize,
}

/// The high-level query plan produced by the parser.
/// Each variant is a logical operation the executor will carry out.
#[derive(Debug, Clone)]
pub enum QueryPlan {
    /// Scan all nodes with the given label.
    ScanNodes(String),
    /// Filter results using a predicate.
    Filter(Box<QueryPlan>, Predicate),
    /// Traverse from source nodes via a relationship type with hop range.
    Traverse {
        source: Box<QueryPlan>,
        rel_type: Option<String>,
        min_hops: usize,
        max_hops: usize,
    },
    /// Project specific aliases into the result set.
    Project(Box<QueryPlan>, Vec<String>),
    /// Match a full (node)-[rel]->(node) path pattern.
    PathMatch {
        from_label: Option<String>,
        from_props: HashMap<String, Value>,
        rel_type: Option<String>,
        min_hops: usize,
        max_hops: usize,
        to_label: Option<String>,
        where_pred: Option<Predicate>,
        return_aliases: Vec<String>,
        from_alias: String,
        to_alias: String,
    },
}

/// Errors from the mini Cypher parser.
#[derive(Debug)]
pub enum ParseError {
    UnexpectedToken(String),
    MissingClause(String),
}

impl std::fmt::Display for ParseError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ParseError::UnexpectedToken(t) => write!(f, "Unexpected token: {}", t),
            ParseError::MissingClause(c) => write!(f, "Missing clause: {}", c),
        }
    }
}

/// Parse a Cypher-lite query string into a `QueryPlan`.
///
/// Supported grammar (simplified):
/// ```
/// query        ::= match_clause [where_clause] return_clause
/// match_clause ::= "MATCH" pattern
/// pattern      ::= node_pat ["-[" rel_pat "]->" node_pat]
/// node_pat     ::= "(" alias [":" label] ["{" prop_list "}"] ")"
/// rel_pat      ::= [alias] [":" type ["*" int ".." int]]
/// where_clause ::= "WHERE" alias "." prop "=" value
/// return_clause::= "RETURN" alias ["," alias]*
/// ```
pub fn parse(query: &str) -> Result<QueryPlan, ParseError> {
    let tokens = tokenize(query);
    let mut pos = 0;

    // Expect MATCH
    expect_keyword(&tokens, &mut pos, "MATCH")?;

    // Parse from-node pattern: (alias[:Label][{props}])
    let from_node = parse_node_pattern(&tokens, &mut pos)?;

    // Check whether there's a relationship pattern next
    if pos < tokens.len() && tokens[pos] == "-[" {
        // Relationship: -[alias:TYPE*min..max]->
        pos += 1; // consume "-["
        let rel = parse_rel_pattern(&tokens, &mut pos)?;
        // Expect "]->"
        if pos < tokens.len() && tokens[pos] == "]->" {
            pos += 1;
        }
        // Parse to-node pattern
        let to_node = parse_node_pattern(&tokens, &mut pos)?;

        // Optional WHERE clause
        let where_pred = if pos < tokens.len() && tokens[pos].eq_ignore_ascii_case("WHERE") {
            pos += 1;
            Some(parse_predicate(&tokens, &mut pos)?)
        } else {
            None
        };

        // RETURN
        expect_keyword(&tokens, &mut pos, "RETURN")?;
        let return_aliases = parse_return_list(&tokens, &mut pos);

        return Ok(QueryPlan::PathMatch {
            from_label: from_node.label,
            from_props: from_node.props,
            rel_type: rel.rel_type,
            min_hops: rel.min_hops,
            max_hops: rel.max_hops,
            to_label: to_node.label,
            where_pred,
            return_aliases,
            from_alias: from_node.alias,
            to_alias: to_node.alias,
        });
    }

    // Simple node-only query: MATCH (n:Label [{props}]) [WHERE ...] RETURN n
    let where_pred = if pos < tokens.len() && tokens[pos].eq_ignore_ascii_case("WHERE") {
        pos += 1;
        Some(parse_predicate(&tokens, &mut pos)?)
    } else {
        None
    };

    expect_keyword(&tokens, &mut pos, "RETURN")?;
    let return_aliases = parse_return_list(&tokens, &mut pos);

    let label = from_node.label.ok_or_else(|| {
        ParseError::MissingClause("node label required for MATCH".into())
    })?;

    let mut plan: QueryPlan = QueryPlan::ScanNodes(label);

    // Apply inline property filters
    for (prop, val) in from_node.props {
        plan = QueryPlan::Filter(
            Box::new(plan),
            Predicate::Eq(from_node.alias.clone(), prop, val),
        );
    }

    // Apply WHERE predicate
    if let Some(pred) = where_pred {
        plan = QueryPlan::Filter(Box::new(plan), pred);
    }

    plan = QueryPlan::Project(Box::new(plan), return_aliases);
    Ok(plan)
}

// ── Tokenizer ─────────────────────────────────────────────────────────────────

fn tokenize(input: &str) -> Vec<String> {
    let mut tokens = Vec::new();
    let mut chars = input.chars().peekable();

    while let Some(&c) = chars.peek() {
        match c {
            ' ' | '\t' | '\n' | '\r' => {
                chars.next();
            }
            '(' | ')' | '{' | '}' | ',' | '.' | '=' | ':' | '*' | '>' => {
                tokens.push(c.to_string());
                chars.next();
            }
            '-' => {
                chars.next();
                if chars.peek() == Some(&'[') {
                    chars.next();
                    tokens.push("-[".to_string());
                } else {
                    tokens.push("-".to_string());
                }
            }
            ']' => {
                chars.next();
                if chars.peek() == Some(&'-') {
                    chars.next();
                    if chars.peek() == Some(&'>') {
                        chars.next();
                        tokens.push("]->".to_string());
                    } else {
                        tokens.push("]-".to_string());
                    }
                } else {
                    tokens.push("]".to_string());
                }
            }
            '"' => {
                chars.next();
                let mut s = String::new();
                while let Some(&ch) = chars.peek() {
                    if ch == '"' {
                        chars.next();
                        break;
                    }
                    s.push(ch);
                    chars.next();
                }
                tokens.push(format!("\"{}\"", s));
            }
            '.' if {
                // Peek-ahead: if followed by '.', it's the ".." range separator
                let mut tmp = chars.clone();
                tmp.next();
                tmp.peek() == Some(&'.')
            } => {
                chars.next();
                chars.next();
                tokens.push("..".to_string());
            }
            _ if c.is_alphanumeric() || c == '_' => {
                let mut word = String::new();
                while let Some(&ch) = chars.peek() {
                    if ch.is_alphanumeric() || ch == '_' {
                        word.push(ch);
                        chars.next();
                    } else {
                        break;
                    }
                }
                tokens.push(word);
            }
            _ => {
                chars.next();
            }
        }
    }
    tokens
}

// ── Parser helpers ────────────────────────────────────────────────────────────

fn expect_keyword(tokens: &[String], pos: &mut usize, kw: &str) -> Result<(), ParseError> {
    if *pos < tokens.len() && tokens[*pos].eq_ignore_ascii_case(kw) {
        *pos += 1;
        Ok(())
    } else {
        let got = tokens.get(*pos).cloned().unwrap_or_else(|| "<EOF>".into());
        Err(ParseError::UnexpectedToken(format!("expected '{}', got '{}'", kw, got)))
    }
}

fn parse_node_pattern(tokens: &[String], pos: &mut usize) -> Result<NodePattern, ParseError> {
    // Expect '('
    if *pos < tokens.len() && tokens[*pos] == "(" {
        *pos += 1;
    } else {
        let got = tokens.get(*pos).cloned().unwrap_or_default();
        return Err(ParseError::UnexpectedToken(format!("expected '(', got '{}'", got)));
    }

    // alias
    let alias = if *pos < tokens.len() && tokens[*pos] != ")" && tokens[*pos] != ":" {
        let a = tokens[*pos].clone();
        *pos += 1;
        a
    } else {
        String::new()
    };

    // optional :Label
    let label = if *pos < tokens.len() && tokens[*pos] == ":" {
        *pos += 1;
        if *pos < tokens.len() {
            let l = tokens[*pos].clone();
            *pos += 1;
            Some(l)
        } else {
            None
        }
    } else {
        None
    };

    // optional {props}
    let mut props = HashMap::new();
    if *pos < tokens.len() && tokens[*pos] == "{" {
        *pos += 1;
        while *pos < tokens.len() && tokens[*pos] != "}" {
            let key = tokens[*pos].clone();
            *pos += 1;
            // ":"
            if *pos < tokens.len() && tokens[*pos] == ":" {
                *pos += 1;
            }
            // value (string literal or bare word)
            let val = parse_literal(tokens, pos);
            props.insert(key, val);
            // optional comma
            if *pos < tokens.len() && tokens[*pos] == "," {
                *pos += 1;
            }
        }
        if *pos < tokens.len() && tokens[*pos] == "}" {
            *pos += 1;
        }
    }

    // Expect ')'
    if *pos < tokens.len() && tokens[*pos] == ")" {
        *pos += 1;
    }

    Ok(NodePattern { alias, label, props })
}

fn parse_rel_pattern(tokens: &[String], pos: &mut usize) -> Result<RelPattern, ParseError> {
    // Optional alias before ":"
    let alias = if *pos < tokens.len() && tokens[*pos] != ":" && tokens[*pos] != "]->" && tokens[*pos] != "]" {
        let a = tokens[*pos].clone();
        *pos += 1;
        Some(a)
    } else {
        None
    };

    // optional :TYPE
    let rel_type = if *pos < tokens.len() && tokens[*pos] == ":" {
        *pos += 1;
        if *pos < tokens.len() && tokens[*pos] != "*" && tokens[*pos] != "]->" && tokens[*pos] != "]" {
            let t = tokens[*pos].clone();
            *pos += 1;
            Some(t)
        } else {
            None
        }
    } else {
        None
    };

    // optional *min..max
    let (min_hops, max_hops) = if *pos < tokens.len() && tokens[*pos] == "*" {
        *pos += 1;
        let min = parse_int(tokens, pos).unwrap_or(1) as usize;
        // ".."
        if *pos < tokens.len() && tokens[*pos] == ".." {
            *pos += 1;
        }
        let max = parse_int(tokens, pos).unwrap_or(min as i64) as usize;
        (min, max)
    } else {
        (1, 1)
    };

    Ok(RelPattern { alias, rel_type, min_hops, max_hops })
}

fn parse_predicate(tokens: &[String], pos: &mut usize) -> Result<Predicate, ParseError> {
    // alias.prop = value
    let alias = tokens.get(*pos).cloned().unwrap_or_default();
    *pos += 1;
    // "."
    if *pos < tokens.len() && tokens[*pos] == "." {
        *pos += 1;
    }
    let prop = tokens.get(*pos).cloned().unwrap_or_default();
    *pos += 1;
    // "="
    if *pos < tokens.len() && tokens[*pos] == "=" {
        *pos += 1;
    }
    let val = parse_literal(tokens, pos);
    Ok(Predicate::Eq(alias, prop, val))
}

fn parse_return_list(tokens: &[String], pos: &mut usize) -> Vec<String> {
    let mut aliases = Vec::new();
    while *pos < tokens.len() {
        let tok = &tokens[*pos];
        if tok == "," {
            *pos += 1;
            continue;
        }
        aliases.push(tok.clone());
        *pos += 1;
    }
    aliases
}

fn parse_literal(tokens: &[String], pos: &mut usize) -> Value {
    if *pos >= tokens.len() {
        return Value::String(String::new());
    }
    let tok = tokens[*pos].clone();
    *pos += 1;

    if tok.starts_with('"') && tok.ends_with('"') {
        Value::String(tok[1..tok.len() - 1].to_string())
    } else if let Ok(i) = tok.parse::<i64>() {
        Value::Int(i)
    } else if let Ok(f) = tok.parse::<f64>() {
        Value::Float(f)
    } else if tok == "true" {
        Value::Bool(true)
    } else if tok == "false" {
        Value::Bool(false)
    } else {
        Value::String(tok)
    }
}

fn parse_int(tokens: &[String], pos: &mut usize) -> Option<i64> {
    if *pos < tokens.len() {
        if let Ok(n) = tokens[*pos].parse::<i64>() {
            *pos += 1;
            return Some(n);
        }
    }
    None
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_match_all_nodes_of_label() {
        let plan = parse("MATCH (n:Person) RETURN n").unwrap();
        match plan {
            QueryPlan::Project(inner, aliases) => {
                assert!(aliases.contains(&"n".to_string()));
                match *inner {
                    QueryPlan::ScanNodes(label) => assert_eq!(label, "Person"),
                    _ => panic!("expected ScanNodes"),
                }
            }
            _ => panic!("expected Project"),
        }
    }

    #[test]
    fn test_parse_match_with_property_filter() {
        let plan = parse(r#"MATCH (n:Person {name: "Alice"}) RETURN n"#).unwrap();
        match plan {
            QueryPlan::Project(inner, _) => {
                match *inner {
                    QueryPlan::Filter(scan, pred) => {
                        match *scan {
                            QueryPlan::ScanNodes(label) => assert_eq!(label, "Person"),
                            _ => panic!("expected ScanNodes"),
                        }
                        match pred {
                            Predicate::Eq(_, prop, val) => {
                                assert_eq!(prop, "name");
                                assert_eq!(val, Value::String("Alice".into()));
                            }
                        }
                    }
                    _ => panic!("expected Filter"),
                }
            }
            _ => panic!("expected Project"),
        }
    }

    #[test]
    fn test_parse_simple_relationship() {
        let plan = parse("MATCH (n:Person)-[r:KNOWS]->(m:Person) RETURN n, m").unwrap();
        match plan {
            QueryPlan::PathMatch { rel_type, min_hops, max_hops, return_aliases, .. } => {
                assert_eq!(rel_type, Some("KNOWS".to_string()));
                assert_eq!(min_hops, 1);
                assert_eq!(max_hops, 1);
                assert!(return_aliases.contains(&"n".to_string()));
                assert!(return_aliases.contains(&"m".to_string()));
            }
            _ => panic!("expected PathMatch"),
        }
    }

    #[test]
    fn test_parse_variable_length_path() {
        let plan = parse(r#"MATCH (n:Person)-[:KNOWS*1..3]->(m:Person) WHERE n.name = "Alice" RETURN m"#).unwrap();
        match plan {
            QueryPlan::PathMatch { min_hops, max_hops, where_pred, .. } => {
                assert_eq!(min_hops, 1);
                assert_eq!(max_hops, 3);
                assert!(where_pred.is_some());
            }
            _ => panic!("expected PathMatch"),
        }
    }

    #[test]
    fn test_parse_where_clause() {
        let plan = parse(r#"MATCH (n:Person) WHERE n.name = "Alice" RETURN n"#).unwrap();
        match plan {
            QueryPlan::Project(inner, _) => match *inner {
                QueryPlan::Filter(_, pred) => match pred {
                    Predicate::Eq(alias, prop, val) => {
                        assert_eq!(alias, "n");
                        assert_eq!(prop, "name");
                        assert_eq!(val, Value::String("Alice".into()));
                    }
                },
                _ => panic!("expected Filter"),
            },
            _ => panic!("expected Project"),
        }
    }

    #[test]
    fn test_parse_return_projects_correct_columns() {
        let plan = parse("MATCH (n:Person)-[r:KNOWS]->(m:Person) RETURN n, m").unwrap();
        match plan {
            QueryPlan::PathMatch { return_aliases, .. } => {
                assert_eq!(return_aliases.len(), 2);
                assert!(return_aliases.contains(&"n".to_string()));
                assert!(return_aliases.contains(&"m".to_string()));
            }
            _ => panic!("expected PathMatch"),
        }
    }
}
