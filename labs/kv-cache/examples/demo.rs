//! Quick demo: v0 HashMap + TTL, then v1 RESP server echo.
//! Run with: cargo run --example demo

use std::time::Duration;
use kv_cache::{v0, v1};

#[tokio::main]
async fn main() {
    // ── v0: pure in-memory cache ──────────────────────────────────────────────
    println!("=== v0 — in-memory cache with lazy TTL ===");
    {
        let mut cache = v0::Cache::new();

        cache.set("session:abc".into(), "user_id=42".into(), Some(300));
        cache.set("rate_limit:42".into(), "47".into(), Some(60));
        cache.set("order:1001".into(), "status=paid".into(), None);

        println!("GET session:abc  => {:?}", cache.get("session:abc"));
        println!("GET order:1001   => {:?}", cache.get("order:1001"));
        println!("TTL session:abc  => {}s", cache.ttl("session:abc"));
        println!("TTL order:1001   => {} (no expiry = -1)", cache.ttl("order:1001"));
        println!("EXISTS order:1001 => {}", cache.exists("order:1001"));

        // Simulate an expired entry (TTL = 0 means already elapsed)
        cache.set("token:xyz".into(), "secret".into(), Some(0));
        println!(
            "GET token:xyz (TTL=0, expired) => {:?}  [lazy eviction]",
            cache.get("token:xyz")
        );

        println!("Keys in store after lazy eviction: {}", cache.len());
    }

    // ── v1: RESP server (background task) ────────────────────────────────────
    println!("\n=== v1 — RESP server on :6380 ===");
    {
        let shared = v1::new_shared_cache();
        let c = shared.clone();
        let addr = "127.0.0.1:6380";

        // Spawn the server in a background task
        tokio::spawn(async move {
            v1::serve(addr, c).await.expect("server failed");
        });

        // Give it a moment to bind
        tokio::time::sleep(Duration::from_millis(50)).await;

        // Drive it with a raw TCP connection (same wire format as Jedis)
        use tokio::io::{AsyncReadExt, AsyncWriteExt};
        use tokio::net::TcpStream;

        let mut stream = TcpStream::connect(addr).await.expect("connect");

        // PING — both inline and array form
        stream.write_all(b"PING\r\n").await.unwrap();
        let mut buf = [0u8; 64];
        let n = stream.read(&mut buf).await.unwrap();
        println!("PING (inline)    => {:?}", std::str::from_utf8(&buf[..n]).unwrap().trim());

        // SET key value
        let set_cmd = b"*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$5\r\nAlice\r\n";
        stream.write_all(set_cmd).await.unwrap();
        let n = stream.read(&mut buf).await.unwrap();
        println!("SET name Alice   => {:?}", std::str::from_utf8(&buf[..n]).unwrap().trim());

        // GET key
        let get_cmd = b"*2\r\n$3\r\nGET\r\n$4\r\nname\r\n";
        stream.write_all(get_cmd).await.unwrap();
        let n = stream.read(&mut buf).await.unwrap();
        println!("GET name         => {:?}", std::str::from_utf8(&buf[..n]).unwrap().trim());

        // SET with EX flag
        let set_ex = b"*5\r\n$3\r\nSET\r\n$7\r\nsession\r\n$3\r\nabc\r\n$2\r\nEX\r\n$3\r\n300\r\n";
        stream.write_all(set_ex).await.unwrap();
        let n = stream.read(&mut buf).await.unwrap();
        println!("SET session abc EX 300 => {:?}", std::str::from_utf8(&buf[..n]).unwrap().trim());

        // TTL
        let ttl_cmd = b"*2\r\n$3\r\nTTL\r\n$7\r\nsession\r\n";
        stream.write_all(ttl_cmd).await.unwrap();
        let n = stream.read(&mut buf).await.unwrap();
        println!("TTL session      => {:?}", std::str::from_utf8(&buf[..n]).unwrap().trim());

        // GET missing key → null bulk
        let miss_cmd = b"*2\r\n$3\r\nGET\r\n$6\r\nnobody\r\n";
        stream.write_all(miss_cmd).await.unwrap();
        let n = stream.read(&mut buf).await.unwrap();
        println!("GET nobody       => {:?} ($-1 = null bulk = Redis miss)", std::str::from_utf8(&buf[..n]).unwrap().trim());
    }

    println!("\n=== Jedis 5.x compatibility ===");
    println!("The server above speaks the same RESP2 wire protocol as Redis.");
    println!("Connect Jedis to localhost:6380 — it will not know the difference:");
    println!();
    println!("  JedisPool pool = new JedisPool(\"localhost\", 6380);");
    println!("  try (Jedis jedis = pool.getResource()) {{");
    println!("      jedis.set(\"hello\", \"world\");   // → +OK");
    println!("      String v = jedis.get(\"hello\");  // → world");
    println!("      jedis.expire(\"hello\", 60);      // → :1");
    println!("  }}");
    println!();
    println!("See labs/kv-cache/java-integration for the full Spring Boot demo.");
}
