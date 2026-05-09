// lexer.rs — SQL tokenizer
//
// Converts raw SQL text into a flat Vec<Token>.  Tokens are consumed by the
// recursive descent parser in parser.rs.  Unknown characters produce a
// LexError rather than silently skipping — fail fast on typos.

use crate::{SqlError, SqlResult};

/// Every distinct token the SQL subset needs.
#[derive(Debug, Clone, PartialEq)]
pub enum Token {
    // Keywords
    Select,
    From,
    Where,
    Insert,
    Into,
    Values,
    Create,
    Table,
    Index,
    On,
    Begin,
    Commit,
    Rollback,
    Order,
    By,
    Asc,
    Desc,
    Limit,
    Offset,
    Join,
    Inner,
    And,
    Or,
    Not,
    Null,
    True,
    False,

    // Punctuation
    Star,        // *
    Comma,       // ,
    Dot,         // .
    Semicolon,   // ;
    LParen,      // (
    RParen,      // )

    // Comparison / arithmetic operators
    Eq,          // =
    NotEq,       // !=  or <>
    Lt,          // <
    Gt,          // >
    LtEq,        // <=
    GtEq,        // >=

    // Literals
    IntLit(i64),
    StrLit(String),
    BoolLit(bool),

    // Identifiers (table names, column names)
    Ident(String),

    // End of input
    Eof,
}

pub struct Lexer {
    input: Vec<char>,
    pos: usize,
}

impl Lexer {
    pub fn new(input: &str) -> Self {
        Lexer { input: input.chars().collect(), pos: 0 }
    }

    /// Tokenize the entire input string.
    pub fn tokenize(&mut self) -> SqlResult<Vec<Token>> {
        let mut tokens = Vec::new();
        loop {
            let tok = self.next_token()?;
            let is_eof = tok == Token::Eof;
            tokens.push(tok);
            if is_eof { break; }
        }
        Ok(tokens)
    }

    fn peek(&self) -> Option<char> {
        self.input.get(self.pos).copied()
    }

    fn advance(&mut self) -> char {
        let c = self.input[self.pos];
        self.pos += 1;
        c
    }

    fn skip_whitespace(&mut self) {
        while self.peek().map(|c| c.is_whitespace()).unwrap_or(false) {
            self.pos += 1;
        }
        // Skip single-line comments (-- ...)
        if self.pos + 1 < self.input.len()
            && self.input[self.pos] == '-'
            && self.input[self.pos + 1] == '-'
        {
            while self.peek().map(|c| c != '\n').unwrap_or(false) {
                self.pos += 1;
            }
            self.skip_whitespace();
        }
    }

    fn read_string(&mut self) -> SqlResult<Token> {
        // Opening ' already consumed by caller
        let mut s = String::new();
        loop {
            match self.peek() {
                None => return Err(SqlError::LexError("unterminated string literal".into())),
                Some('\'') => {
                    self.advance();
                    // Two consecutive quotes = escaped quote inside string
                    if self.peek() == Some('\'') {
                        self.advance();
                        s.push('\'');
                    } else {
                        break;
                    }
                }
                Some(c) => { self.advance(); s.push(c); }
            }
        }
        Ok(Token::StrLit(s))
    }

    fn read_number(&mut self, first: char) -> SqlResult<Token> {
        let mut n = String::from(first);
        while self.peek().map(|c| c.is_ascii_digit()).unwrap_or(false) {
            n.push(self.advance());
        }
        n.parse::<i64>()
            .map(Token::IntLit)
            .map_err(|_| SqlError::LexError(format!("invalid integer: {}", n)))
    }

    fn read_ident(&mut self, first: char) -> Token {
        let mut s = String::from(first);
        while self.peek().map(|c| c.is_alphanumeric() || c == '_').unwrap_or(false) {
            s.push(self.advance());
        }
        keyword_or_ident(s)
    }

    fn next_token(&mut self) -> SqlResult<Token> {
        self.skip_whitespace();
        match self.peek() {
            None => Ok(Token::Eof),
            Some(c) => {
                self.advance();
                match c {
                    '*' => Ok(Token::Star),
                    ',' => Ok(Token::Comma),
                    '.' => Ok(Token::Dot),
                    ';' => Ok(Token::Semicolon),
                    '(' => Ok(Token::LParen),
                    ')' => Ok(Token::RParen),
                    '=' => Ok(Token::Eq),
                    '<' => {
                        if self.peek() == Some('=') { self.advance(); Ok(Token::LtEq) }
                        else if self.peek() == Some('>') { self.advance(); Ok(Token::NotEq) }
                        else { Ok(Token::Lt) }
                    }
                    '>' => {
                        if self.peek() == Some('=') { self.advance(); Ok(Token::GtEq) }
                        else { Ok(Token::Gt) }
                    }
                    '!' => {
                        if self.peek() == Some('=') { self.advance(); Ok(Token::NotEq) }
                        else { Err(SqlError::LexError(format!("unexpected '!' at pos {}", self.pos))) }
                    }
                    '\'' => self.read_string(),
                    d if d.is_ascii_digit() => self.read_number(d),
                    '-' if self.peek().map(|c| c.is_ascii_digit()).unwrap_or(false) => {
                        let tok = self.read_number(self.advance())?;
                        Ok(match tok { Token::IntLit(n) => Token::IntLit(-n), t => t })
                    }
                    a if a.is_alphabetic() || a == '_' => Ok(self.read_ident(a)),
                    other => Err(SqlError::LexError(format!("unexpected character '{}'", other))),
                }
            }
        }
    }
}

fn keyword_or_ident(s: String) -> Token {
    match s.to_uppercase().as_str() {
        "SELECT"   => Token::Select,
        "FROM"     => Token::From,
        "WHERE"    => Token::Where,
        "INSERT"   => Token::Insert,
        "INTO"     => Token::Into,
        "VALUES"   => Token::Values,
        "CREATE"   => Token::Create,
        "TABLE"    => Token::Table,
        "INDEX"    => Token::Index,
        "ON"       => Token::On,
        "BEGIN"    => Token::Begin,
        "COMMIT"   => Token::Commit,
        "ROLLBACK" => Token::Rollback,
        "ORDER"    => Token::Order,
        "BY"       => Token::By,
        "ASC"      => Token::Asc,
        "DESC"     => Token::Desc,
        "LIMIT"    => Token::Limit,
        "OFFSET"   => Token::Offset,
        "JOIN"     => Token::Join,
        "INNER"    => Token::Inner,
        "AND"      => Token::And,
        "OR"       => Token::Or,
        "NOT"      => Token::Not,
        "NULL"     => Token::Null,
        "TRUE"     => Token::BoolLit(true),
        "FALSE"    => Token::BoolLit(false),
        _          => Token::Ident(s),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lex_select_star() {
        let mut l = Lexer::new("SELECT * FROM users WHERE id = 1");
        let toks = l.tokenize().unwrap();
        assert_eq!(toks[0], Token::Select);
        assert_eq!(toks[1], Token::Star);
        assert_eq!(toks[2], Token::From);
        assert_eq!(toks[3], Token::Ident("users".into()));
        assert_eq!(toks[4], Token::Where);
        assert_eq!(toks[6], Token::Eq);
        assert_eq!(toks[7], Token::IntLit(1));
    }

    #[test]
    fn lex_string_literal() {
        let mut l = Lexer::new("INSERT INTO t VALUES ('hello world')");
        let toks = l.tokenize().unwrap();
        assert!(toks.iter().any(|t| *t == Token::StrLit("hello world".into())));
    }

    #[test]
    fn lex_operators() {
        let mut l = Lexer::new("a != b AND c <= d OR e >= f");
        let toks = l.tokenize().unwrap();
        assert!(toks.contains(&Token::NotEq));
        assert!(toks.contains(&Token::LtEq));
        assert!(toks.contains(&Token::GtEq));
    }
}
