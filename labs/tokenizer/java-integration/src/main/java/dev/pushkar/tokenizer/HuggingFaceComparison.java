package dev.pushkar.tokenizer;

import ai.djl.huggingface.tokenizers.HuggingFaceTokenizer;
import ai.djl.huggingface.tokenizers.Encoding;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.nio.file.Path;
import java.util.Arrays;
import java.util.List;

/**
 * Demonstrates vocabulary portability using DJL's HuggingFace tokenizer binding.
 *
 * The key insight: our Python GPT2BPETokenizer saves its vocabulary and merge
 * rules as JSON (via save()). DJL's tokenizer library reads HuggingFace's
 * tokenizer.json format — which is the same structure. So the same vocab file
 * produces identical token IDs in both Python and Java.
 *
 * This is why vocabulary format matters: a tokenizer is only useful if the
 * model trained on those IDs and the serving infrastructure agree on the
 * same ID for every input. HuggingFace's tokenizer.json is the de-facto
 * portability standard.
 *
 * Note: To use this with the Python tokenizer's output, convert the JSON
 * saved by GPT2BPETokenizer.save() to HuggingFace format using the
 * convert_to_hf_format() helper in src/v2_gpt2bpe.py (see comments there).
 * For this demo, we use a pre-built GPT-2 tokenizer from HuggingFace.
 */
@Component
public class HuggingFaceComparison {

    /**
     * Load a HuggingFace tokenizer from a local tokenizer.json file
     * and encode text, returning the token IDs as a list.
     *
     * Demonstrates: if the vocabPath points to a tokenizer.json built from
     * the same merge rules as our Python implementation, the token IDs will
     * match exactly.
     *
     * @param vocabPath  Path to a HuggingFace tokenizer.json file.
     * @param text       Text to tokenize.
     * @return           List of integer token IDs.
     */
    public List<Long> encodeWithHuggingFace(Path vocabPath, String text) throws IOException {
        // DJL loads the tokenizer from the standard HuggingFace JSON format.
        // The file contains: vocab, merges, and special token configuration.
        // This is the same information our Python save() serialises, just in
        // the HuggingFace schema rather than our custom schema.
        try (HuggingFaceTokenizer tokenizer = HuggingFaceTokenizer.newInstance(vocabPath)) {
            Encoding encoding = tokenizer.encode(text);
            long[] ids = encoding.getIds();
            // Convert primitive array to boxed List for Spring compatibility
            Long[] boxed = new Long[ids.length];
            for (int i = 0; i < ids.length; i++) {
                boxed[i] = ids[i];
            }
            return Arrays.asList(boxed);
        }
    }

    /**
     * Decode a list of token IDs back to a string using a HuggingFace tokenizer.
     *
     * @param vocabPath  Path to a HuggingFace tokenizer.json file.
     * @param ids        Token IDs to decode.
     * @return           Decoded string.
     */
    public String decodeWithHuggingFace(Path vocabPath, List<Long> ids) throws IOException {
        try (HuggingFaceTokenizer tokenizer = HuggingFaceTokenizer.newInstance(vocabPath)) {
            long[] primitiveIds = new long[ids.size()];
            for (int i = 0; i < ids.size(); i++) {
                primitiveIds[i] = ids.get(i);
            }
            return tokenizer.decode(primitiveIds, true);
        }
    }

    /**
     * Compare our Python tokenizer's output with the HuggingFace tokenizer's output.
     *
     * If both tokenizers use the same vocabulary and merge rules, they will produce
     * identical token IDs. This is the portability guarantee.
     *
     * @param pythonTokens  Token IDs from our Python server (via TokenizerClient.encode()).
     * @param hfTokens      Token IDs from DJL's HuggingFace binding.
     * @return              true if both token lists are identical, false if they differ.
     */
    public boolean tokensMatch(List<Integer> pythonTokens, List<Long> hfTokens) {
        if (pythonTokens.size() != hfTokens.size()) {
            return false;
        }
        for (int i = 0; i < pythonTokens.size(); i++) {
            if (pythonTokens.get(i).longValue() != hfTokens.get(i)) {
                return false;
            }
        }
        return true;
    }
}
