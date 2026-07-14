// SPDX-License-Identifier: AGPL-3.0-only
package dockerapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
)

func TestScanMultiplexedLogsPreservesStreams(t *testing.T) {
	var input bytes.Buffer
	for _, value := range []struct {
		stream byte
		text   string
	}{{1, "out\n"}, {2, "err\n"}} {
		header := make([]byte, 8)
		header[0] = value.stream
		binary.BigEndian.PutUint32(header[4:], uint32(len(value.text)))
		input.Write(header)
		input.WriteString(value.text)
	}
	var got []string
	err := scanMultiplexedLogs(context.Background(), &input, func(stream, line string) error { got = append(got, stream+":"+line); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "stdout:out" || got[1] != "stderr:err" {
		t.Fatalf("logs=%v", got)
	}
}

func TestScanTTYLogs(t *testing.T) {
	var got []string
	err := scanLogLines(context.Background(), bytes.NewBufferString("one\ntwo\n"), "stdout", func(stream, line string) error { got = append(got, stream+":"+line); return nil })
	if err != nil || len(got) != 2 {
		t.Fatalf("logs=%v err=%v", got, err)
	}
}
