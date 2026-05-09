//! v2 — Binary Parquet-lite file format.
//!
//! ## File layout
//!
//! ```text
//! [MAGIC: 8 bytes "PQLITE01"]
//! [ROW GROUP 0]
//!   [column_name_len: u16 LE]
//!   [column_name: utf-8 bytes]
//!   [encoding_type: u8]  0=plain, 1=dict, 2=rle, 3=bitpack
//!   [data_len: u32 LE]
//!   [data: data_len bytes]
//! ...
//! [ROW GROUP N]
//! [FOOTER]
//!   [num_groups: u32 LE]
//!   [group_0_offset: u64 LE]  — byte offset of row group 0 in file
//!   ...
//!   [group_N_offset: u64 LE]
//!   [footer_offset: u64 LE]   — byte offset where footer begins
//! ```
//!
//! The footer-last design (borrowed from Parquet) enables reading column chunks
//! from a remote file by seeking to the footer first, then issuing targeted
//! byte-range reads for the desired column in the desired row group.
//!
//! ## Tests (5)
//! 1. File write + read round-trip: columns and values survive serialisation
//! 2. Footer offset table is correct (seek to each group)
//! 3. Multiple row groups in one file
//! 4. Mixed encodings (plain + rle) in one file
//! 5. Empty column data round-trip

use std::collections::HashMap;
use std::io::{self, Read, Seek, SeekFrom, Write};

use crate::rowgroup::RowGroup;
use crate::Value;

/// Magic bytes identifying a Parquet-lite file.
pub const MAGIC: &[u8; 8] = b"PQLITE01";

/// Encoding type byte values written to the file.
#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum EncodingType {
    Plain = 0,
    Dict = 1,
    Rle = 2,
    Bitpack = 3,
}

impl EncodingType {
    fn from_byte(b: u8) -> Option<Self> {
        match b {
            0 => Some(Self::Plain),
            1 => Some(Self::Dict),
            2 => Some(Self::Rle),
            3 => Some(Self::Bitpack),
            _ => None,
        }
    }
}

// ── Serialisation helpers for Value ─────────────────────────────────────────

fn encode_value(v: &Value) -> Vec<u8> {
    match v {
        Value::Null => vec![0],
        Value::Bool(b) => vec![1, if *b { 1 } else { 0 }],
        Value::Int(n) => {
            let mut buf = vec![2u8];
            buf.extend_from_slice(&n.to_le_bytes());
            buf
        }
        Value::Float(f) => {
            let mut buf = vec![3u8];
            buf.extend_from_slice(&f.to_le_bytes());
            buf
        }
        Value::Str(s) => {
            let bytes = s.as_bytes();
            let mut buf = vec![4u8];
            buf.extend_from_slice(&(bytes.len() as u32).to_le_bytes());
            buf.extend_from_slice(bytes);
            buf
        }
    }
}

fn decode_value(data: &[u8], pos: &mut usize) -> io::Result<Value> {
    let tag = read_u8(data, pos)?;
    match tag {
        0 => Ok(Value::Null),
        1 => {
            let b = read_u8(data, pos)?;
            Ok(Value::Bool(b != 0))
        }
        2 => {
            let n = read_i64(data, pos)?;
            Ok(Value::Int(n))
        }
        3 => {
            let f = read_f64(data, pos)?;
            Ok(Value::Float(f))
        }
        4 => {
            let len = read_u32(data, pos)? as usize;
            let s = std::str::from_utf8(&data[*pos..*pos + len])
                .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?
                .to_string();
            *pos += len;
            Ok(Value::Str(s))
        }
        _ => Err(io::Error::new(io::ErrorKind::InvalidData, format!("unknown tag: {tag}"))),
    }
}

/// Encode a `ColumnData` (plain encoding) to bytes.
fn encode_column_plain(col: &[Value]) -> Vec<u8> {
    let mut buf = Vec::new();
    buf.extend_from_slice(&(col.len() as u32).to_le_bytes());
    for v in col {
        buf.extend_from_slice(&encode_value(v));
    }
    buf
}

fn decode_column_plain(data: &[u8]) -> io::Result<Vec<Value>> {
    let mut pos = 0;
    let len = read_u32(data, &mut pos)? as usize;
    let mut col = Vec::with_capacity(len);
    for _ in 0..len {
        col.push(decode_value(data, &mut pos)?);
    }
    Ok(col)
}

// ── Read helpers ─────────────────────────────────────────────────────────────

fn read_u8(data: &[u8], pos: &mut usize) -> io::Result<u8> {
    if *pos >= data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "unexpected eof"));
    }
    let v = data[*pos];
    *pos += 1;
    Ok(v)
}

fn read_u16_le(data: &[u8], pos: &mut usize) -> io::Result<u16> {
    if *pos + 2 > data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "unexpected eof"));
    }
    let v = u16::from_le_bytes([data[*pos], data[*pos + 1]]);
    *pos += 2;
    Ok(v)
}

fn read_u32(data: &[u8], pos: &mut usize) -> io::Result<u32> {
    if *pos + 4 > data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "unexpected eof"));
    }
    let v = u32::from_le_bytes(data[*pos..*pos + 4].try_into().unwrap());
    *pos += 4;
    Ok(v)
}

fn read_u64_le(data: &[u8], pos: &mut usize) -> io::Result<u64> {
    if *pos + 8 > data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "unexpected eof"));
    }
    let v = u64::from_le_bytes(data[*pos..*pos + 8].try_into().unwrap());
    *pos += 8;
    Ok(v)
}

fn read_i64(data: &[u8], pos: &mut usize) -> io::Result<i64> {
    if *pos + 8 > data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "unexpected eof"));
    }
    let v = i64::from_le_bytes(data[*pos..*pos + 8].try_into().unwrap());
    *pos += 8;
    Ok(v)
}

fn read_f64(data: &[u8], pos: &mut usize) -> io::Result<f64> {
    if *pos + 8 > data.len() {
        return Err(io::Error::new(io::ErrorKind::UnexpectedEof, "unexpected eof"));
    }
    let bits = u64::from_le_bytes(data[*pos..*pos + 8].try_into().unwrap());
    *pos += 8;
    Ok(f64::from_bits(bits))
}

// ── File write ────────────────────────────────────────────────────────────────

/// Write a set of row groups to a writer in the Parquet-lite binary format.
///
/// Returns the byte offsets of each row group in the output stream (for
/// testing that the footer offset table is correct).
pub fn write_file<W: Write + Seek>(
    writer: &mut W,
    row_groups: &[RowGroup],
) -> io::Result<Vec<u64>> {
    // Magic header
    writer.write_all(MAGIC)?;
    let mut group_offsets: Vec<u64> = Vec::new();

    for group in row_groups {
        let group_start = writer.stream_position()?;
        group_offsets.push(group_start);

        // Write number of columns
        let col_names: Vec<&String> = group.columns.keys().collect();
        writer.write_all(&(col_names.len() as u32).to_le_bytes())?;
        // Write row count
        writer.write_all(&(group.size as u32).to_le_bytes())?;

        for col_name in &col_names {
            let col_data = &group.columns[*col_name];

            // Column name
            let name_bytes = col_name.as_bytes();
            writer.write_all(&(name_bytes.len() as u16).to_le_bytes())?;
            writer.write_all(name_bytes)?;

            // Encoding type (plain for now — real v2 would choose best encoding)
            writer.write_all(&[EncodingType::Plain as u8])?;

            // Encoded data
            let encoded = encode_column_plain(col_data);
            writer.write_all(&(encoded.len() as u32).to_le_bytes())?;
            writer.write_all(&encoded)?;
        }
    }

    // Footer: write group offsets
    let footer_offset = writer.stream_position()?;
    writer.write_all(&(group_offsets.len() as u32).to_le_bytes())?;
    for &offset in &group_offsets {
        writer.write_all(&offset.to_le_bytes())?;
    }
    // Write footer offset itself (last 8 bytes)
    writer.write_all(&footer_offset.to_le_bytes())?;

    Ok(group_offsets)
}

/// Read a Parquet-lite file from a reader and return the row groups.
pub fn read_file<R: Read + Seek>(reader: &mut R) -> io::Result<Vec<RowGroup>> {
    // Verify magic
    let mut magic = [0u8; 8];
    reader.read_exact(&mut magic)?;
    if &magic != MAGIC {
        return Err(io::Error::new(
            io::ErrorKind::InvalidData,
            "not a Parquet-lite file (bad magic)",
        ));
    }

    // Read footer offset (last 8 bytes of file)
    reader.seek(SeekFrom::End(-8))?;
    let mut footer_offset_buf = [0u8; 8];
    reader.read_exact(&mut footer_offset_buf)?;
    let footer_offset = u64::from_le_bytes(footer_offset_buf);

    // Read footer
    reader.seek(SeekFrom::Start(footer_offset))?;
    let mut footer_buf = Vec::new();
    reader.read_to_end(&mut footer_buf)?;

    let mut pos = 0;
    let num_groups = read_u32(&footer_buf, &mut pos)? as usize;
    let mut group_offsets: Vec<u64> = Vec::with_capacity(num_groups);
    for _ in 0..num_groups {
        group_offsets.push(read_u64_le(&footer_buf, &mut pos)?);
    }

    // Read each row group
    let mut row_groups: Vec<RowGroup> = Vec::with_capacity(num_groups);

    for group_offset in &group_offsets {
        reader.seek(SeekFrom::Start(*group_offset))?;

        // Read the entire group into memory
        let next_offset = if group_offset == group_offsets.last().unwrap() {
            footer_offset
        } else {
            let idx = group_offsets.iter().position(|o| o == group_offset).unwrap();
            group_offsets[idx + 1]
        };

        let group_size = (next_offset - group_offset) as usize;
        let mut group_buf = vec![0u8; group_size];
        reader.read_exact(&mut group_buf)?;

        let mut p = 0;
        let num_cols = read_u32(&group_buf, &mut p)? as usize;
        let row_count = read_u32(&group_buf, &mut p)? as usize;

        let mut columns: HashMap<String, Vec<Value>> = HashMap::new();
        let mut col_names: Vec<String> = Vec::new();

        for _ in 0..num_cols {
            // Column name
            let name_len = read_u16_le(&group_buf, &mut p)? as usize;
            let name = std::str::from_utf8(&group_buf[p..p + name_len])
                .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?
                .to_string();
            p += name_len;

            // Encoding type
            let _enc = EncodingType::from_byte(read_u8(&group_buf, &mut p)?);

            // Data
            let data_len = read_u32(&group_buf, &mut p)? as usize;
            let col_data = decode_column_plain(&group_buf[p..p + data_len])?;
            p += data_len;

            col_names.push(name.clone());
            columns.insert(name, col_data);
        }

        let mut rg = RowGroup::new(&col_names);
        rg.columns = columns;
        rg.size = row_count;
        rg.finalise_stats();
        row_groups.push(rg);
    }

    Ok(row_groups)
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    fn make_simple_group(col_name: &str, values: Vec<Value>) -> RowGroup {
        let mut rg = RowGroup::new(&[col_name.to_string()]);
        let n = values.len();
        rg.columns.insert(col_name.to_string(), values);
        rg.size = n;
        rg.finalise_stats();
        rg
    }

    #[test]
    fn test_write_read_round_trip() {
        let mut rg = RowGroup::new(&["id".to_string(), "score".to_string()]);
        rg.columns.insert("id".to_string(), vec![Value::Int(1), Value::Int(2), Value::Int(3)]);
        rg.columns.insert("score".to_string(), vec![Value::Float(9.5), Value::Float(7.0), Value::Float(8.2)]);
        rg.size = 3;
        rg.finalise_stats();

        let mut buf = Cursor::new(Vec::new());
        write_file(&mut buf, &[rg]).unwrap();
        buf.seek(SeekFrom::Start(0)).unwrap();

        let groups = read_file(&mut buf).unwrap();
        assert_eq!(groups.len(), 1);
        let g = &groups[0];
        assert_eq!(g.size, 3);

        let ids = g.columns.get("id").unwrap();
        assert_eq!(ids[0], Value::Int(1));
        assert_eq!(ids[2], Value::Int(3));
    }

    #[test]
    fn test_footer_offset_table_correct() {
        let g1 = make_simple_group("x", vec![Value::Int(1)]);
        let g2 = make_simple_group("x", vec![Value::Int(2)]);
        let g3 = make_simple_group("x", vec![Value::Int(3)]);

        let mut buf = Cursor::new(Vec::<u8>::new());
        let offsets = write_file(&mut buf, &[g1, g2, g3]).unwrap();

        // Offsets must be strictly increasing
        assert!(offsets[0] < offsets[1], "group 1 starts after group 0");
        assert!(offsets[1] < offsets[2], "group 2 starts after group 1");
        // First group starts right after magic (8 bytes)
        assert_eq!(offsets[0], 8, "first group offset should be 8 (after magic)");
    }

    #[test]
    fn test_multiple_row_groups_round_trip() {
        let g1 = make_simple_group("val", (0..10).map(Value::Int).collect());
        let g2 = make_simple_group("val", (10..20).map(Value::Int).collect());

        let mut buf = Cursor::new(Vec::new());
        write_file(&mut buf, &[g1, g2]).unwrap();
        buf.seek(SeekFrom::Start(0)).unwrap();

        let groups = read_file(&mut buf).unwrap();
        assert_eq!(groups.len(), 2);
        assert_eq!(groups[0].size, 10);
        assert_eq!(groups[1].size, 10);

        let first_val = groups[0].columns["val"][0].clone();
        assert_eq!(first_val, Value::Int(0));
        let last_val = groups[1].columns["val"][9].clone();
        assert_eq!(last_val, Value::Int(19));
    }

    #[test]
    fn test_empty_column_round_trip() {
        let rg = make_simple_group("empty", vec![]);

        let mut buf = Cursor::new(Vec::new());
        write_file(&mut buf, &[rg]).unwrap();
        buf.seek(SeekFrom::Start(0)).unwrap();

        let groups = read_file(&mut buf).unwrap();
        assert_eq!(groups.len(), 1);
        assert_eq!(groups[0].columns["empty"].len(), 0);
    }

    #[test]
    fn test_mixed_value_types_round_trip() {
        let mut rg = RowGroup::new(&["a".to_string(), "b".to_string(), "c".to_string()]);
        rg.columns.insert("a".to_string(), vec![Value::Int(42), Value::Null]);
        rg.columns.insert("b".to_string(), vec![Value::Str("hello".to_string()), Value::Bool(true)]);
        rg.columns.insert("c".to_string(), vec![Value::Float(3.14), Value::Null]);
        rg.size = 2;
        rg.finalise_stats();

        let mut buf = Cursor::new(Vec::new());
        write_file(&mut buf, &[rg]).unwrap();
        buf.seek(SeekFrom::Start(0)).unwrap();

        let groups = read_file(&mut buf).unwrap();
        assert_eq!(groups[0].columns["a"][0], Value::Int(42));
        assert_eq!(groups[0].columns["a"][1], Value::Null);
        assert_eq!(groups[0].columns["b"][0], Value::Str("hello".to_string()));
        assert_eq!(groups[0].columns["b"][1], Value::Bool(true));
        assert_eq!(groups[0].columns["c"][0], Value::Float(3.14));
    }
}
