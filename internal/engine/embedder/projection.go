package embedder

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// projection holds a dense linear layer loaded from a safetensors file.
// It projects vectors from inDim to outDim via matrix-vector multiplication
// (no bias, identity activation).
type projection struct {
	weights []float32 // row-major [outDim, inDim]
	inDim   int
	outDim  int
}

// loadProjection reads a safetensors file containing a single "linear.weight"
// tensor of dtype F32.
func loadProjection(path string) (*projection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("projection: %w", err)
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("projection: file too small: %d bytes", len(data))
	}

	// Parse safetensors header: 8-byte LE uint64 header length, then JSON.
	headerLen := binary.LittleEndian.Uint64(data[:8])
	if uint64(len(data)) < 8+headerLen {
		return nil, fmt.Errorf("projection: header length %d exceeds file size", headerLen)
	}

	var header map[string]json.RawMessage
	if err := json.Unmarshal(data[8:8+headerLen], &header); err != nil {
		return nil, fmt.Errorf("projection: failed to parse header: %w", err)
	}

	raw, ok := header["linear.weight"]
	if !ok {
		return nil, fmt.Errorf("projection: tensor 'linear.weight' not found in header")
	}

	var meta struct {
		Dtype       string  `json:"dtype"`
		Shape       []int   `json:"shape"`
		DataOffsets [2]int  `json:"data_offsets"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("projection: failed to parse tensor metadata: %w", err)
	}

	if meta.Dtype != "F32" {
		return nil, fmt.Errorf("projection: expected dtype F32, got %s", meta.Dtype)
	}
	if len(meta.Shape) != 2 {
		return nil, fmt.Errorf("projection: expected 2D tensor, got shape %v", meta.Shape)
	}

	outDim := meta.Shape[0]
	inDim := meta.Shape[1]
	numFloats := outDim * inDim
	expectedBytes := numFloats * 4

	dataStart := int(8 + headerLen) + meta.DataOffsets[0]
	dataEnd := int(8 + headerLen) + meta.DataOffsets[1]
	if dataEnd-dataStart != expectedBytes {
		return nil, fmt.Errorf("projection: data size %d doesn't match shape %v",
			dataEnd-dataStart, meta.Shape)
	}
	if dataEnd > len(data) {
		return nil, fmt.Errorf("projection: data range [%d:%d] exceeds file size %d",
			dataStart, dataEnd, len(data))
	}

	// Reinterpret raw bytes as float32 slice.
	weights := make([]float32, numFloats)
	for i := range weights {
		bits := binary.LittleEndian.Uint32(data[dataStart+i*4 : dataStart+i*4+4])
		weights[i] = math.Float32frombits(bits)
	}

	return &projection{
		weights: weights,
		inDim:   inDim,
		outDim:  outDim,
	}, nil
}

// apply projects a single vector from inDim to outDim.
func (p *projection) apply(vec []float32) []float32 {
	out := make([]float32, p.outDim)
	for i := 0; i < p.outDim; i++ {
		row := p.weights[i*p.inDim : (i+1)*p.inDim]
		var sum float32
		for j, w := range row {
			sum += w * vec[j]
		}
		out[i] = sum
	}
	return out
}
