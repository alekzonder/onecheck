package module

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func testModule(collect CollectFunc) Module {
	return Module{
		Description: Description{
			Name:        "fake",
			Version:     "1.0.0",
			Description: "fake module",
			Fields:      map[string]FieldInfo{"text": {Type: TypeString, Description: "some text"}},
		},
		Collect: collect,
	}
}

func doCheck(t *testing.T, m Module, req string) (Response, int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(m, []string{"check"}, strings.NewReader(req), &out, &errOut)
	var resp Response
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
			t.Fatalf("stdout is not JSON: %v: %s", err, out.String())
		}
	}
	return resp, code, errOut.String()
}

func TestRunDescribe(t *testing.T) {
	var out bytes.Buffer
	code := Run(testModule(nil), []string{"describe"}, strings.NewReader(""), &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	var d Description
	if err := json.Unmarshal(out.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Protocol != ProtocolVersion || d.Name != "fake" || d.Fields["text"].Type != TypeString {
		t.Errorf("unexpected description: %+v", d)
	}
}

func TestRunCheck(t *testing.T) {
	m := testModule(func(_ context.Context, req Request) (map[string]any, error) {
		if !req.Wants("text") || req.Wants("other") {
			t.Errorf("Wants is wrong for %v", req.Fields)
		}
		return map[string]any{"text": "hello", "n": 3}, nil
	})
	resp, code, _ := doCheck(t, m, `{"protocol":1,"rule":"r","params":{},"fields":["text"],"max_field_size":100,"timeout_ms":1000}`)
	if code != 0 || resp.Error != nil {
		t.Fatalf("code=%d err=%v", code, resp.Error)
	}
	if resp.Fields["text"] != "hello" || resp.Fields["n"] != float64(3) {
		t.Errorf("fields = %v", resp.Fields)
	}
	if resp.Truncated == nil || len(resp.Truncated) != 0 {
		t.Errorf("truncated must be an empty list, got %v", resp.Truncated)
	}
}

func TestRunCollectError(t *testing.T) {
	m := testModule(func(context.Context, Request) (map[string]any, error) {
		return nil, errors.New("connection refused")
	})
	resp, code, _ := doCheck(t, m, `{"protocol":1,"rule":"r","params":{}}`)
	if code != 0 {
		t.Errorf("collect error must exit 0, got %d", code)
	}
	if resp.Error == nil || resp.Error.Message != "connection refused" {
		t.Errorf("error = %+v", resp.Error)
	}
}

func TestRunTruncates(t *testing.T) {
	m := testModule(func(context.Context, Request) (map[string]any, error) {
		return map[string]any{"text": "0123456789", "short": "ab"}, nil
	})
	resp, _, _ := doCheck(t, m, `{"protocol":1,"rule":"r","params":{},"max_field_size":4}`)
	if resp.Fields["text"] != "0123" {
		t.Errorf("text = %q, want cut to 4 bytes", resp.Fields["text"])
	}
	if len(resp.Truncated) != 1 || resp.Truncated[0] != "text" {
		t.Errorf("truncated = %v", resp.Truncated)
	}
}

func TestRunProtocolMismatch(t *testing.T) {
	_, code, errOut := doCheck(t, testModule(nil), `{"protocol":99,"rule":"r","params":{}}`)
	if code == 0 || !strings.Contains(errOut, "protocol") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var errOut bytes.Buffer
	if code := Run(testModule(nil), []string{"bogus"}, strings.NewReader(""), &bytes.Buffer{}, &errOut); code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "describe") {
		t.Errorf("usage missing: %q", errOut.String())
	}
}

func TestRunCollectErrorKeepsPartialFields(t *testing.T) {
	m := testModule(func(context.Context, Request) (map[string]any, error) {
		return map[string]any{"stdout": "so far"}, errors.New("timed out")
	})
	resp, _, _ := doCheck(t, m, `{"protocol":1,"rule":"r","params":{}}`)
	if resp.Error == nil || resp.Error.Message != "timed out" {
		t.Fatalf("error = %+v", resp.Error)
	}
	if resp.Fields["stdout"] != "so far" {
		t.Errorf("partial fields were dropped: %v", resp.Fields)
	}
}
