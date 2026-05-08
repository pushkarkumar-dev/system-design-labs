# transformer-from-scratch
# Stage layout:
#   v0_attention.py      — scaled dot-product attention + multi-head attention
#   v1_transformer_block.py — feed-forward, layer norm, residuals, positional encoding
#   v2_gpt.py            — GPT-style autoregressive transformer with causal masking
#   train.py             — TinyShakespeare training loop
#   server.py            — FastAPI inference server
