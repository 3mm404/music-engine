package player

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestMonoMixHandlesClippingAndPartialReads(t *testing.T) {
	input := []int16{32767, 32767, -32768, -32768, 12000, -4000, 100, -100}
	var source bytes.Buffer
	for _, v := range input {
		binary.Write(&source, binary.LittleEndian, v)
	}
	r := &monoReader{source: &source}
	var got []byte
	for {
		b := make([]byte, 3)
		n, err := r.Read(b)
		got = append(got, b[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []int16{32767, 32767, -32768, -32768, 4000, 4000, 0, 0}
	for i, v := range want {
		if actual := int16(binary.LittleEndian.Uint16(got[i*2:])); actual != v {
			t.Fatalf("sample %d: %d != %d", i, actual, v)
		}
	}
}
