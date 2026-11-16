package tunnel

import (
	"bytes"
	"testing"
)

func TestOpenFrameRoundTrip(t *testing.T) {
	original := Open{StreamID: 42, Protocol: ProtoTCP, Address: "example.com:443"}
	decoded, err := DecodeOpen(EncodeOpen(original))
	if err != nil {
		t.Fatal(err)
	}
	if decoded != original {
		t.Fatalf("decoded %+v, want %+v", decoded, original)
	}
}

func TestOpenAckRoundTrip(t *testing.T) {
	id, status, err := DecodeOpenAck(EncodeOpenAck(7, StatusOK))
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 || status != StatusOK {
		t.Fatalf("decoded %d/%d, want 7/0", id, status)
	}
}

func TestDataFrameRoundTrip(t *testing.T) {
	payload := []byte("hello tunnel")
	id, data, err := DecodeData(EncodeData(9, payload))
	if err != nil {
		t.Fatal(err)
	}
	if id != 9 {
		t.Fatalf("stream id = %d, want 9", id)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload = %q, want %q", data, payload)
	}
}

func TestCloseFrameRoundTrip(t *testing.T) {
	id, reason, err := DecodeClose(EncodeClose(3, StatusError))
	if err != nil {
		t.Fatal(err)
	}
	if id != 3 || reason != StatusError {
		t.Fatalf("decoded %d/%d, want 3/%d", id, reason, StatusError)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := DecodeOpen([]byte{0x01, 0x02}); err == nil {
		t.Error("a short open frame was accepted")
	}
	if _, err := DecodeOpen(EncodeData(1, nil)); err == nil {
		t.Error("a frame with the wrong type was accepted")
	}
	if _, _, err := DecodeData([]byte{0x04}); err == nil {
		t.Error("a short data frame was accepted")
	}
	if _, _, err := DecodeData(EncodeData(1, nil)); err != nil {
		t.Error("an empty data frame was rejected")
	}
	if _, err := Type(nil); err == nil {
		t.Error("an empty payload was accepted")
	}
}
