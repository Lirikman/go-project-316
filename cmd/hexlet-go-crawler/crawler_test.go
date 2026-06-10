package main

import (
	"bytes"
	"code"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

// MockClient структура для подмены клиента в тестах.
type MockClient struct {
	MockDo func(req *http.Request) (*http.Response, error)
}

// Do перенаправляет вызов на нашу mock-функцию
func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	if m.MockDo != nil {
		return m.MockDo(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(""))}, nil
}

// параметры для выполнения запросов в тестах
var testOpts = code.Options{
	Client:      *http.Client,
	URL:         "https://example.com",
	Depth:       1,
	Retries:     1,
	Delay:       0 * time.Second,
	Timeout:     15 * time.Second,
	UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	Concurrency: 4,
	IndentJSON:  true,
}

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name        string
		mockClient  *MockClient
		expected    string
		expectError bool
	}{
		{
			name: "Success Scenario",
			mockClient: &MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewBufferString("Test Title")),
					}, nil
				},
			},
			expected:    "Test Title",
			expectError: false,
		},
		{
			name: "HTTP 500 Error",
			mockClient: &MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(bytes.NewBufferString("")),
					}, nil
				},
			},
			expected:    "",
			expectError: true,
		},
		{
			name: "Network Failure",
			mockClient: &MockClient{
				MockDo: func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("connection refused")
				},
			},
			expected:    "",
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			opts.Client := tt.mockClient
			res, err := code.Analyze(ctx, testOpts)
			// Проверяем ошибку
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected error state: got %v, expected error: %v", err, tt.expectError)
			}

			// Проверяем значение
			if title != tt.expected {
				t.Fatalf("unexpected title: got %q, expected %q", title, tt.expected)
			}
		})
	}
}
