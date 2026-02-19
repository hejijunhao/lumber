package embedder

import (
	"os"
	"reflect"
	"testing"
)

const testVocabPath = "../../../models/vocab.txt"

func skipIfNoVocab(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(testVocabPath); os.IsNotExist(err) {
		t.Skip("vocab.txt not found; run 'make download-model' first")
	}
}

func testTokenizer(t *testing.T) *tokenizer {
	t.Helper()
	skipIfNoVocab(t)
	tok, err := newTokenizer(testVocabPath)
	if err != nil {
		t.Fatalf("failed to create tokenizer: %v", err)
	}
	return tok
}

func TestVocabLoad(t *testing.T) {
	skipIfNoVocab(t)
	v, err := loadVocab(testVocabPath)
	if err != nil {
		t.Fatalf("failed to load vocab: %v", err)
	}
	if v.size() != 30522 {
		t.Errorf("expected 30522 tokens, got %d", v.size())
	}
	if v.padID != 0 {
		t.Errorf("expected [PAD]=0, got %d", v.padID)
	}
	if v.unkID != 100 {
		t.Errorf("expected [UNK]=100, got %d", v.unkID)
	}
	if v.clsID != 101 {
		t.Errorf("expected [CLS]=101, got %d", v.clsID)
	}
	if v.sepID != 102 {
		t.Errorf("expected [SEP]=102, got %d", v.sepID)
	}
}

// Reference tokenizations generated with HuggingFace BertTokenizer.
var tokenizeTests = []struct {
	name string
	text string
	ids  []int64 // expected input_ids (non-padding portion)
}{
	{
		name: "simple",
		text: "hello world",
		ids:  []int64{101, 7592, 2088, 102},
	},
	{
		name: "empty string",
		text: "",
		ids:  []int64{101, 102},
	},
	{
		name: "log line with punctuation and numbers",
		text: "ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)",
		ids:  []int64{101, 7561, 1031, 16798, 2575, 1011, 6185, 1011, 2539, 2260, 1024, 4002, 1024, 4002, 1033, 5198, 2121, 7903, 2063, 1517, 4434, 4188, 1006, 3677, 1027, 16962, 1011, 3078, 1010, 3417, 1027, 5139, 16703, 1007, 102},
	},
	{
		name: "IP address and duration",
		text: "Connection timeout to 10.0.0.1:5432 after 30s",
		ids:  []int64{101, 4434, 2051, 5833, 2000, 2184, 1012, 1014, 1012, 1014, 1012, 1015, 1024, 5139, 16703, 2044, 2382, 2015, 102},
	},
	{
		name: "accented characters stripped",
		text: "café résumé naïve",
		ids:  []int64{101, 7668, 13746, 15743, 102},
	},
	{
		name: "chinese characters",
		text: "你好世界",
		ids:  []int64{101, 100, 100, 1745, 100, 102},
	},
	{
		name: "mixed punctuation brackets",
		text: "a]b[c",
		ids:  []int64{101, 1037, 1033, 1038, 1031, 1039, 102},
	},
}

func TestTokenize(t *testing.T) {
	tok := testTokenizer(t)

	for _, tc := range tokenizeTests {
		t.Run(tc.name, func(t *testing.T) {
			ids, mask, typeIDs := tok.tokenize(tc.text)

			// Check that non-padding IDs match expected.
			realLen := len(tc.ids)
			gotIDs := ids[:realLen]
			if !reflect.DeepEqual(gotIDs, tc.ids) {
				t.Errorf("input_ids mismatch\n  want: %v\n  got:  %v", tc.ids, gotIDs)
			}

			// Attention mask: 1s for real tokens, 0s for padding.
			for i := 0; i < realLen; i++ {
				if mask[i] != 1 {
					t.Errorf("attention_mask[%d] = %d, want 1", i, mask[i])
				}
			}
			for i := realLen; i < maxSeqLen; i++ {
				if mask[i] != 0 {
					t.Errorf("attention_mask[%d] = %d, want 0 (padding)", i, mask[i])
				}
			}

			// Padding IDs should be 0.
			for i := realLen; i < maxSeqLen; i++ {
				if ids[i] != 0 {
					t.Errorf("input_ids[%d] = %d, want 0 (padding)", i, ids[i])
				}
			}

			// Token type IDs should be all zeros.
			for i := 0; i < maxSeqLen; i++ {
				if typeIDs[i] != 0 {
					t.Errorf("token_type_ids[%d] = %d, want 0", i, typeIDs[i])
				}
			}

			// Output length should be exactly maxSeqLen.
			if len(ids) != maxSeqLen || len(mask) != maxSeqLen || len(typeIDs) != maxSeqLen {
				t.Errorf("expected output length %d, got ids=%d mask=%d typeIDs=%d",
					maxSeqLen, len(ids), len(mask), len(typeIDs))
			}
		})
	}
}

func TestTokenizeTruncation(t *testing.T) {
	tok := testTokenizer(t)

	// Generate a string that will produce more than 126 tokens (maxSeqLen - 2
	// for [CLS] and [SEP]). Each space-separated word is one basic token.
	words := make([]byte, 0, 200*4)
	for i := 0; i < 200; i++ {
		if i > 0 {
			words = append(words, ' ')
		}
		words = append(words, 'a')
	}

	ids, mask, _ := tok.tokenize(string(words))

	if len(ids) != maxSeqLen {
		t.Fatalf("expected %d IDs, got %d", maxSeqLen, len(ids))
	}

	// First token is [CLS].
	if ids[0] != 101 {
		t.Errorf("expected [CLS] at position 0, got %d", ids[0])
	}

	// Last real token is [SEP] — count real tokens from mask.
	realCount := 0
	for _, m := range mask {
		if m == 1 {
			realCount++
		}
	}
	if realCount != maxSeqLen {
		t.Errorf("expected %d real tokens after truncation, got %d", maxSeqLen, realCount)
	}
	if ids[maxSeqLen-1] != 102 {
		t.Errorf("expected [SEP] at position %d, got %d", maxSeqLen-1, ids[maxSeqLen-1])
	}
}

func TestTokenizeBatch(t *testing.T) {
	tok := testTokenizer(t)

	texts := []string{
		"hello world",      // 4 real tokens
		"connection error", // 3 real tokens (connection + error + CLS + SEP = 4, but let's see)
	}
	result := tok.tokenizeBatch(texts)

	if result.batchSize != 2 {
		t.Fatalf("expected batchSize=2, got %d", result.batchSize)
	}

	// SeqLen should be the max of the two sequences' real token counts.
	total := result.batchSize * result.seqLen
	if int64(len(result.inputIDs)) != total {
		t.Fatalf("expected %d input_ids, got %d", total, len(result.inputIDs))
	}
	if int64(len(result.attentionMask)) != total {
		t.Fatalf("expected %d attention_mask values, got %d", total, len(result.attentionMask))
	}
	if int64(len(result.tokenTypeIDs)) != total {
		t.Fatalf("expected %d token_type_ids, got %d", total, len(result.tokenTypeIDs))
	}

	// First sequence starts with [CLS].
	if result.inputIDs[0] != 101 {
		t.Errorf("first sequence should start with [CLS]=101, got %d", result.inputIDs[0])
	}

	// Second sequence starts with [CLS].
	offset := result.seqLen
	if result.inputIDs[offset] != 101 {
		t.Errorf("second sequence should start with [CLS]=101, got %d", result.inputIDs[offset])
	}
}

func TestTokenizeBatchEmpty(t *testing.T) {
	tok := testTokenizer(t)

	result := tok.tokenizeBatch(nil)
	if result.batchSize != 0 {
		t.Errorf("expected batchSize=0 for empty input, got %d", result.batchSize)
	}
}
