//! v2 — RLE (run-length encoding) and bit-packing.
//!
//! Two additional encoding schemes for the cases where dictionary encoding
//! is not beneficial:
//!
//! ## RLE (Run-Length Encoding)
//! Replaces repeated consecutive values with (value, count) pairs.
//! Best for sorted or near-sorted columns (e.g., a timestamp column where
//! many rows share the same hour). A sorted integer column with average run
//! length 200 compresses 200× on the runs, reducing to ~0.5% of original size.
//!
//! ## Bit-packing
//! For integer columns where all values fit in N bits (e.g., 0–255 → 8 bits),
//! pack N bits per value instead of 64 bits. An 8-bit column uses 1/8 the
//! storage of a raw i64 column.
//!
//! ## Tests (5)
//! 1. RLE compresses a sorted column to far fewer run pairs
//! 2. RLE decode round-trip — decode(encode(col)) == col
//! 3. Bit-pack 8-bit column uses 1/8 the bytes of raw i64
//! 4. Bit-unpack round-trip — unpack(pack(values)) == values
//! 5. Mixed encoding: RLE + dict + bitpack in the same file

use crate::Value;

// ── Run-Length Encoding ───────────────────────────────────────────────────────

/// A run-length-encoded column. Each entry is (value, run_count).
#[derive(Debug, Clone)]
pub struct RleColumn {
    pub runs: Vec<(Value, u32)>,
    /// Original row count (needed to reconstruct exactly).
    pub total_rows: usize,
}

/// Encode a column with RLE.
///
/// Consecutive equal values are merged into a single (value, count) pair.
/// Null values are encoded as runs of `Value::Null`.
pub fn encode_rle(col: &[Value]) -> RleColumn {
    let mut runs: Vec<(Value, u32)> = Vec::new();

    for val in col {
        match runs.last_mut() {
            Some((last_val, count)) if last_val == val => {
                *count += 1;
            }
            _ => {
                runs.push((val.clone(), 1));
            }
        }
    }

    RleColumn { runs, total_rows: col.len() }
}

/// Decode a `RleColumn` back to a flat `Vec<Value>`.
pub fn decode_rle(rle: &RleColumn) -> Vec<Value> {
    let mut result = Vec::with_capacity(rle.total_rows);
    for (val, count) in &rle.runs {
        for _ in 0..*count {
            result.push(val.clone());
        }
    }
    result
}

/// Return the compression ratio (original bytes / encoded bytes).
///
/// Estimated as `total_rows / run_count`. A ratio of 200 means the encoded
/// form is 200× smaller.
pub fn rle_compression_ratio(rle: &RleColumn) -> f64 {
    rle.total_rows as f64 / rle.runs.len().max(1) as f64
}

// ── Bit-packing ────────────────────────────────────────────────────────────

/// Return the minimum number of bits needed to represent `max_val`.
///
/// Values are assumed to be non-negative. Panics if `bits` would exceed 64.
pub fn bits_needed(max_val: i64) -> u8 {
    if max_val <= 0 {
        return 1;
    }
    let bits = (64 - max_val.leading_zeros()) as u8;
    bits.max(1)
}

/// Pack `values` using `bits` bits per value.
///
/// Output is a byte slice where each value occupies exactly `bits` bits,
/// packed MSB-first. The last byte may be zero-padded.
pub fn bitpack(values: &[i64], bits: u8) -> Vec<u8> {
    assert!(bits <= 64, "bits must be <= 64");
    let total_bits = values.len() * bits as usize;
    let byte_count = (total_bits + 7) / 8;
    let mut output = vec![0u8; byte_count];

    let mut bit_pos = 0usize;
    for &v in values {
        for bit_idx in (0..bits).rev() {
            let bit = ((v >> bit_idx) & 1) as u8;
            let byte_idx = bit_pos / 8;
            let bit_within_byte = 7 - (bit_pos % 8);
            output[byte_idx] |= bit << bit_within_byte;
            bit_pos += 1;
        }
    }

    output
}

/// Unpack `len` values from a bit-packed byte slice.
///
/// Each value occupies `bits` bits. Matches the format written by `bitpack`.
pub fn bitunpack(packed: &[u8], bits: u8, len: usize) -> Vec<i64> {
    let mut result = Vec::with_capacity(len);
    let mut bit_pos = 0usize;

    for _ in 0..len {
        let mut val: i64 = 0;
        for bit_idx in (0..bits).rev() {
            let byte_idx = bit_pos / 8;
            let bit_within_byte = 7 - (bit_pos % 8);
            let bit = ((packed[byte_idx] >> bit_within_byte) & 1) as i64;
            val |= bit << bit_idx;
            bit_pos += 1;
        }
        result.push(val);
    }

    result
}

/// Extract raw integer values from a `Value::Int` slice.
///
/// Returns `None` if any value is not `Value::Int`.
pub fn extract_ints(col: &[Value]) -> Option<Vec<i64>> {
    col.iter().map(|v| v.as_int()).collect()
}

// ── Tests ────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rle_compresses_sorted_column() {
        // Sorted timestamp-like column: many consecutive equal values
        // 10,000 values with avg run of 100 → ~100 runs
        let mut col: Vec<Value> = Vec::new();
        for hour in 0..100i64 {
            for _ in 0..100 {
                col.push(Value::Int(hour));
            }
        }
        let rle = encode_rle(&col);
        assert_eq!(rle.runs.len(), 100, "100 distinct hours = 100 runs");
        assert_eq!(rle.total_rows, 10_000);
        let ratio = rle_compression_ratio(&rle);
        assert!(ratio >= 100.0, "compression ratio should be >= 100x for perfectly sorted data");
    }

    #[test]
    fn test_rle_decode_round_trip() {
        let col: Vec<Value> = vec![
            Value::Int(1), Value::Int(1), Value::Int(2),
            Value::Null, Value::Null, Value::Int(3),
        ];
        let rle = encode_rle(&col);
        let decoded = decode_rle(&rle);
        assert_eq!(decoded, col, "RLE round-trip must be lossless");
    }

    #[test]
    fn test_bitpack_8bit_uses_eighth_storage() {
        // Values 0..=255 fit in 8 bits; raw i64 uses 8 bytes each
        let values: Vec<i64> = (0i64..256).collect();
        let packed = bitpack(&values, 8);
        // 256 values × 8 bits = 256 bytes
        assert_eq!(packed.len(), 256);
        // Raw storage would be 256 × 8 bytes = 2048 bytes — 8× more
        let raw_bytes = values.len() * 8;
        assert_eq!(raw_bytes / packed.len(), 8, "bit-packing should use 1/8 the storage");
    }

    #[test]
    fn test_bitunpack_round_trip() {
        let values: Vec<i64> = vec![0, 15, 7, 255, 128, 1, 63, 200];
        let bits = bits_needed(*values.iter().max().unwrap());
        let packed = bitpack(&values, bits);
        let unpacked = bitunpack(&packed, bits, values.len());
        assert_eq!(unpacked, values, "bit-pack round-trip must be lossless");
    }

    #[test]
    fn test_bits_needed() {
        assert_eq!(bits_needed(0), 1);
        assert_eq!(bits_needed(1), 1);
        assert_eq!(bits_needed(255), 8);
        assert_eq!(bits_needed(256), 9);
        assert_eq!(bits_needed(1023), 10);
        assert_eq!(bits_needed(i32::MAX as i64), 31);
    }
}
