package main

import (
	"code"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// mockServ позволяет гибко настраивать ответы для тестов
type mockServ struct {
	transportFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockServ) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.transportFunc(req)
}

func newMockClient(fn func(req *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{
		Transport: &mockServ{transportFunc: fn},
	}
}

// Helper для HTTP-ответа
func stringResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// func TestAnalyze_SuccessAndDepth(t *testing.T) {
// 	client := newMockClient(func(req *http.Request) (*http.Response, error) {
// 		switch req.URL.String() {
// 		case "https://example.com":
// 			return stringResponse(200, `<html><a href="https://example.com"></a></html>`), nil
// 		case "https://example.com":
// 			return stringResponse(200, `<html><a href="https://example.com"></a></html>`), nil
// 		case "https://example.com":
// 			return stringResponse(200, `<html>The end</html>`), nil
// 		default:
// 			return stringResponse(404, "Not Found"), nil
// 		}
// 	})

// 	optsTest := code.Options{
// 		URL:         "https://example.com",
// 		Depth:       2, // Должен зайти на /page1, но проигнорировать /page2
// 		Concurrency: 2,
// 		HTTPClient:  client,
// 	}

// 	data, err := code.Analyze(context.Background(), optsTest)
// 	if err != nil {
// 		t.Fatalf("unexpected error: %v", err)
// 	}

// 	var reports []code.Page
// 	if err := json.Unmarshal(data, &reports); err != nil {
// 		t.Fatalf("failed to unmarshal result: %v", err)
// 	}

// 	if len(reports) != 2 {
// 		t.Errorf("expected 2 pages, got %d", len(reports))
// 	}
// }

func TestAnalyze_ContextCancellation(t *testing.T) {
	client := newMockClient(func(req *http.Request) (*http.Response, error) {
		time.Sleep(50 * time.Millisecond) // имититация долгого запроса
		return stringResponse(200, "OK"), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	opts := code.Options{
		URL:         "https://ya.ru",
		Depth:       1,
		Concurrency: 1,
		HTTPClient:  client,
	}

	_, err := code.Analyze(ctx, opts)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected deadline exceeded error, got %v", err)
	}
}
