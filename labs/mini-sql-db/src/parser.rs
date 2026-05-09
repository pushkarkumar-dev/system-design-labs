// parser.rs — Recursive descent parser
//
// Consumes a token stream (Vec<Token>) from the lexer and produces a Vec<Stmt>.
// One pass over the token vector; O(n) in the number of tokens.
//
// Grammar (simplified):
//
//   stmt      ::= select | insert | create_table | create_index | begin | commit | rollback
//   select    ::= SELECT col_list FROM ident [join] [where] [order_by] [limit] [offset]
//   insert    ::= INSERT INTO ident [(col_list)] VALUES (expr_list) [, (expr_list)]*
//   create_t  ::= CREATE TABLE ident ( col_def [, col_def]* )
//   create_i  ::= CREATE INDEX ident ON ident ( ident )
//   col_def   ::= ident type_name
//   where     ::= WHERE expr
//   expr      ::= and_expr [OR and_expr]*
//   and_expr  ::= cmp_expr [AND cmp_expr]*
//   cmp_expr  ::= atom (= | != | < | > | <= | >=) atom | atom
//   atom      ::= literal | ident | ident.ident | (expr)

use crate::{SqlError, SqlResult};
use crate::lexer::Token;
use crate::ast::*;

pub struct Parser {
    tokens: Vec<Token>,
    pos: usize,
}

impl Parser {
    pub fn new(tokens: Vec<Token>) -> Self {
        Parser { tokens, pos: 0 }
    }

    // ── Token stream helpers ─────────────────────────────────────────────────

    fn peek(&self) -> &Token {
        self.tokens.get(self.pos).unwrap_or(&Token::Eof)
    }

    fn advance(&mut self) -> &Token {
        let t = &self.tokens[self.pos];
        self.pos += 1;
        t
    }

    fn expect(&mut self, expected: &Token) -> SqlResult<()> {
        if self.peek() == expected {
            self.advance();
            Ok(())
        } else {
            Err(SqlError::ParseError(format!(
                "expected {:?} but got {:?}", expected, self.peek()
            )))
        }
    }

    fn expect_ident(&mut self) -> SqlResult<String> {
        match self.peek().clone() {
            Token::Ident(s) => { self.advance(); Ok(s) }
            other => Err(SqlError::ParseError(format!("expected identifier, got {:?}", other))),
        }
    }

    fn maybe(&mut self, tok: &Token) -> bool {
        if self.peek() == tok { self.advance(); true } else { false }
    }

    // ── Public entry point ───────────────────────────────────────────────────

    pub fn parse(&mut self) -> SqlResult<Vec<Stmt>> {
        let mut stmts = Vec::new();
        while self.peek() != &Token::Eof {
            stmts.push(self.parse_stmt()?);
            self.maybe(&Token::Semicolon);
        }
        Ok(stmts)
    }

    fn parse_stmt(&mut self) -> SqlResult<Stmt> {
        match self.peek().clone() {
            Token::Select   => self.parse_select().map(Stmt::Select),
            Token::Insert   => self.parse_insert().map(Stmt::Insert),
            Token::Create   => self.parse_create(),
            Token::Begin    => { self.advance(); Ok(Stmt::Begin) }
            Token::Commit   => { self.advance(); Ok(Stmt::Commit) }
            Token::Rollback => { self.advance(); Ok(Stmt::Rollback) }
            other => Err(SqlError::ParseError(format!("unexpected token: {:?}", other))),
        }
    }

    // ── SELECT ───────────────────────────────────────────────────────────────

    fn parse_select(&mut self) -> SqlResult<SelectStmt> {
        self.expect(&Token::Select)?;
        let columns = self.parse_select_cols()?;
        self.expect(&Token::From)?;
        let from = self.expect_ident()?;

        // Optional JOIN
        let join = if matches!(self.peek(), Token::Inner | Token::Join) {
            Some(self.parse_join()?)
        } else {
            None
        };

        // Optional WHERE
        let where_clause = if self.maybe(&Token::Where) {
            Some(self.parse_expr()?)
        } else {
            None
        };

        // Optional ORDER BY
        let order_by = if self.peek() == &Token::Order {
            self.advance();
            self.expect(&Token::By)?;
            let col = self.expect_ident()?;
            let ascending = !self.maybe(&Token::Desc);
            if self.peek() == &Token::Asc { self.advance(); }
            Some(OrderBy { column: col, ascending })
        } else {
            None
        };

        // Optional LIMIT / OFFSET
        let limit = if self.maybe(&Token::Limit) {
            match self.advance().clone() {
                Token::IntLit(n) => Some(n as usize),
                t => return Err(SqlError::ParseError(format!("expected integer after LIMIT, got {:?}", t))),
            }
        } else { None };

        let offset = if self.maybe(&Token::Offset) {
            match self.advance().clone() {
                Token::IntLit(n) => Some(n as usize),
                t => return Err(SqlError::ParseError(format!("expected integer after OFFSET, got {:?}", t))),
            }
        } else { None };

        Ok(SelectStmt { columns, from, join, where_clause, order_by, limit, offset })
    }

    fn parse_select_cols(&mut self) -> SqlResult<Vec<SelectCol>> {
        let mut cols = Vec::new();
        loop {
            match self.peek().clone() {
                Token::Star => { self.advance(); cols.push(SelectCol::Star); }
                Token::Ident(name) => {
                    self.advance();
                    if self.maybe(&Token::Dot) {
                        let col = self.expect_ident()?;
                        cols.push(SelectCol::Qualified(name, col));
                    } else {
                        cols.push(SelectCol::Named(name));
                    }
                }
                _ => break,
            }
            if !self.maybe(&Token::Comma) { break; }
        }
        if cols.is_empty() {
            return Err(SqlError::ParseError("SELECT list is empty".into()));
        }
        Ok(cols)
    }

    fn parse_join(&mut self) -> SqlResult<JoinClause> {
        if self.peek() == &Token::Inner { self.advance(); }
        self.expect(&Token::Join)?;
        let table = self.expect_ident()?;
        self.expect(&Token::On)?;
        let left = self.expect_ident()?;
        self.expect(&Token::Dot)?;
        let left_col = self.expect_ident()?;
        self.expect(&Token::Eq)?;
        let right = self.expect_ident()?;
        self.expect(&Token::Dot)?;
        let right_col = self.expect_ident()?;
        // We ignore `left` table qualifier here and store column names only
        let _ = right; // suppress unused warning
        Ok(JoinClause { table, left_col, right_col })
    }

    // ── INSERT ───────────────────────────────────────────────────────────────

    fn parse_insert(&mut self) -> SqlResult<InsertStmt> {
        self.expect(&Token::Insert)?;
        self.expect(&Token::Into)?;
        let table = self.expect_ident()?;

        // Optional column list
        let columns = if self.maybe(&Token::LParen) {
            let mut cols = Vec::new();
            loop {
                cols.push(self.expect_ident()?);
                if !self.maybe(&Token::Comma) { break; }
            }
            self.expect(&Token::RParen)?;
            cols
        } else {
            Vec::new()
        };

        self.expect(&Token::Values)?;

        // One or more row tuples
        let mut rows = Vec::new();
        loop {
            self.expect(&Token::LParen)?;
            let mut vals = Vec::new();
            loop {
                vals.push(self.parse_atom()?);
                if !self.maybe(&Token::Comma) { break; }
            }
            self.expect(&Token::RParen)?;
            rows.push(vals);
            if !self.maybe(&Token::Comma) { break; }
        }

        Ok(InsertStmt { table, columns, rows })
    }

    // ── CREATE ───────────────────────────────────────────────────────────────

    fn parse_create(&mut self) -> SqlResult<Stmt> {
        self.expect(&Token::Create)?;
        match self.peek().clone() {
            Token::Table => { self.advance(); self.parse_create_table().map(Stmt::CreateTable) }
            Token::Index => { self.advance(); self.parse_create_index().map(Stmt::CreateIndex) }
            other => Err(SqlError::ParseError(format!("expected TABLE or INDEX after CREATE, got {:?}", other))),
        }
    }

    fn parse_create_table(&mut self) -> SqlResult<CreateTableStmt> {
        let name = self.expect_ident()?;
        self.expect(&Token::LParen)?;
        let mut columns = Vec::new();
        loop {
            let col_name = self.expect_ident()?;
            let type_tok = self.advance().clone();
            let col_type = match &type_tok {
                Token::Ident(t) => t.clone(),
                other => return Err(SqlError::ParseError(format!("expected type name, got {:?}", other))),
            };
            columns.push(ColDef { name: col_name, col_type, nullable: true });
            if !self.maybe(&Token::Comma) { break; }
        }
        self.expect(&Token::RParen)?;
        Ok(CreateTableStmt { name, columns })
    }

    fn parse_create_index(&mut self) -> SqlResult<CreateIndexStmt> {
        let index_name = self.expect_ident()?;
        self.expect(&Token::On)?;
        let table = self.expect_ident()?;
        self.expect(&Token::LParen)?;
        let column = self.expect_ident()?;
        self.expect(&Token::RParen)?;
        Ok(CreateIndexStmt { index_name, table, column })
    }

    // ── Expressions ──────────────────────────────────────────────────────────

    fn parse_expr(&mut self) -> SqlResult<Expr> {
        self.parse_or_expr()
    }

    fn parse_or_expr(&mut self) -> SqlResult<Expr> {
        let mut left = self.parse_and_expr()?;
        while self.peek() == &Token::Or {
            self.advance();
            let right = self.parse_and_expr()?;
            left = Expr::BinOp {
                left: Box::new(left),
                op: BinOpKind::Or,
                right: Box::new(right),
            };
        }
        Ok(left)
    }

    fn parse_and_expr(&mut self) -> SqlResult<Expr> {
        let mut left = self.parse_not_expr()?;
        while self.peek() == &Token::And {
            self.advance();
            let right = self.parse_not_expr()?;
            left = Expr::BinOp {
                left: Box::new(left),
                op: BinOpKind::And,
                right: Box::new(right),
            };
        }
        Ok(left)
    }

    fn parse_not_expr(&mut self) -> SqlResult<Expr> {
        if self.maybe(&Token::Not) {
            let inner = self.parse_cmp_expr()?;
            Ok(Expr::Not(Box::new(inner)))
        } else {
            self.parse_cmp_expr()
        }
    }

    fn parse_cmp_expr(&mut self) -> SqlResult<Expr> {
        let left = self.parse_atom()?;
        let op = match self.peek() {
            Token::Eq    => BinOpKind::Eq,
            Token::NotEq => BinOpKind::NotEq,
            Token::Lt    => BinOpKind::Lt,
            Token::Gt    => BinOpKind::Gt,
            Token::LtEq  => BinOpKind::LtEq,
            Token::GtEq  => BinOpKind::GtEq,
            _            => return Ok(left),
        };
        self.advance();
        let right = self.parse_atom()?;
        Ok(Expr::BinOp { left: Box::new(left), op, right: Box::new(right) })
    }

    fn parse_atom(&mut self) -> SqlResult<Expr> {
        match self.peek().clone() {
            Token::IntLit(n)  => { self.advance(); Ok(Expr::Literal(crate::Value::Int(n))) }
            Token::StrLit(s)  => { self.advance(); Ok(Expr::Literal(crate::Value::Text(s))) }
            Token::BoolLit(b) => { self.advance(); Ok(Expr::Literal(crate::Value::Bool(b))) }
            Token::Null       => { self.advance(); Ok(Expr::Literal(crate::Value::Null)) }
            Token::Ident(name) => {
                self.advance();
                if self.maybe(&Token::Dot) {
                    let col = self.expect_ident()?;
                    Ok(Expr::QualifiedColumn(name, col))
                } else {
                    Ok(Expr::Column(name))
                }
            }
            Token::LParen => {
                self.advance();
                let e = self.parse_expr()?;
                self.expect(&Token::RParen)?;
                Ok(e)
            }
            other => Err(SqlError::ParseError(format!("unexpected token in expression: {:?}", other))),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::lexer::Lexer;

    fn parse(sql: &str) -> Vec<Stmt> {
        let mut l = Lexer::new(sql);
        let toks = l.tokenize().unwrap();
        let mut p = Parser::new(toks);
        p.parse().unwrap()
    }

    #[test]
    fn parse_select_star() {
        let stmts = parse("SELECT * FROM users");
        assert_eq!(stmts.len(), 1);
        assert!(matches!(&stmts[0], Stmt::Select(s) if s.from == "users"));
    }

    #[test]
    fn parse_select_where() {
        let stmts = parse("SELECT name FROM users WHERE id = 1");
        assert!(matches!(&stmts[0], Stmt::Select(s) if s.where_clause.is_some()));
    }

    #[test]
    fn parse_insert() {
        let stmts = parse("INSERT INTO users (id, name) VALUES (1, 'Alice')");
        assert!(matches!(&stmts[0], Stmt::Insert(i) if i.table == "users"));
    }

    #[test]
    fn parse_create_table() {
        let stmts = parse("CREATE TABLE products (id INT, name TEXT, price INT)");
        assert!(matches!(&stmts[0], Stmt::CreateTable(c) if c.columns.len() == 3));
    }

    #[test]
    fn parse_create_index() {
        let stmts = parse("CREATE INDEX idx_name ON users (name)");
        assert!(matches!(&stmts[0], Stmt::CreateIndex(c) if c.table == "users"));
    }

    #[test]
    fn parse_order_limit_offset() {
        let stmts = parse("SELECT id FROM t ORDER BY id DESC LIMIT 5 OFFSET 10");
        if let Stmt::Select(s) = &stmts[0] {
            assert_eq!(s.limit, Some(5));
            assert_eq!(s.offset, Some(10));
            assert!(s.order_by.as_ref().map(|o| !o.ascending).unwrap_or(false));
        } else { panic!("not a select"); }
    }
}
