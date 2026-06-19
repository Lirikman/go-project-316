package main

import (
	"code"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// cоздание подменного HTTP-сервера
func NewMockServer() *httptest.Server {
	mux := http.NewServeMux()

	// главная страница с несколькими ссылками
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		structHtml := `<!DOCTYPE html>
			<html>
			<head>
				<meta charset="UTF-8">
   	 			<title>General page</title>
			</head>
			<body>
    			<h1>Golang forever!</h1>
    			<a href="/about">About Golang</a>
				<a href="/contacts">Contacts</a>
				<a href="https://external.com">External Link</a>
			</body>
			</html>`
		fmt.Fprintln(w, structHtml)
	})

	// страница /about: содержит ссылку на более глубокий уровень
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		structHtml := `<!DOCTYPE html>
			<html>
				<head>
					<meta charset="UTF-8">
					<title>General page</title>
					<meta name="description" content="Golang is the best programming language">
				</head>
			<body>
				<h1>About</h1>
				<a href="/about/team">Our Team</a>
			</body>
			</html>`
		fmt.Fprintln(w, structHtml)
	})

	// страница глубокого уровня
	mux.HandleFunc("/about/team", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `<html><body><p>Some data</p></body></html>`)
	})

	// битый эндпоинт
	mux.HandleFunc("/contacts", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// запуск тестового сервера
	return httptest.NewServer(mux)
}

func TestCrawlerDepth(t *testing.T) {
	// запускаем подменный сервер
	mockServer := NewMockServer()

	// останавливаем сервер после теста
	defer mockServer.Close()

	// настраиваем подменный HTTP-клиент, перенаправляющий запросы
	client := mockServer.Client()

	testOpts := code.Options{
		HTTPClient:  client,
		URL:         mockServer.URL,
		UserAgent:   "TestBot/1.0",
		Depth:       2,
		Concurrency: 3,
		IndentJSON:  true,
	}

	ctx := context.Background()
	jsonData, err := code.Analyze(ctx, testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// десериализуем результат для проверки структуры и контента
	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}
	// проверям количество полученных страниц
	expectedCount := 3
	if len(results.Pages) != expectedCount {
		t.Errorf("Expected %d results, got %d. Dump: %s", expectedCount, len(results.Pages), string(jsonData))
	}
}

func TestCrawlerParsingPage(t *testing.T) {
	// запускаем подменный сервер
	mockServer := NewMockServer()

	// останавливаем сервер после теста
	defer mockServer.Close()

	// настраиваем подменный HTTP-клиент, перенаправляющий запросы
	client := mockServer.Client()

	testOpts := code.Options{
		HTTPClient:  client,
		URL:         mockServer.URL,
		UserAgent:   "TestBot/1.0",
		Depth:       2,
		Concurrency: 3,
		IndentJSON:  true,
	}

	ctx := context.Background()
	jsonData, err := code.Analyze(ctx, testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// десериализуем результат для проверки структуры и контента
	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}
	var wantUrl []string
	for _, link := range results.Pages {
		wantUrl = append(wantUrl, link.URL)
	}
	// проверяем ссылки полученных страниц
	gotUrl := []string{fmt.Sprintf("%s/", mockServer.URL), fmt.Sprintf("%s/about", mockServer.URL), fmt.Sprintf("%s/about/team", mockServer.URL)}

	if slices.Equal(gotUrl, wantUrl) {
		t.Errorf("Expected %v results, got %v. Dump: %s", wantUrl, gotUrl, string(jsonData))
	}
}

func TestCrawlerStatusOK(t *testing.T) {
	// запускаем подменный сервер
	mockServer := NewMockServer()

	// останавливаем сервер после теста
	defer mockServer.Close()

	// настраиваем подменный HTTP-клиент, перенаправляющий запросы
	client := mockServer.Client()

	testOpts := code.Options{
		HTTPClient:  client,
		URL:         mockServer.URL,
		UserAgent:   "TestBot/1.0",
		Depth:       1,
		Concurrency: 2,
		IndentJSON:  true,
	}

	ctx := context.Background()
	jsonData, err := code.Analyze(ctx, testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// десериализуем результат для проверки структуры и контента
	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}
	// проверяем статус код страницы
	gotPageAbout := results.Pages[1].HTTPStatus
	wantPageStatus := 200

	if wantPageStatus != gotPageAbout {
		t.Errorf("Expected status %v results, got %v", wantPageStatus, gotPageAbout)
	}
}

func TestCrawlerBrokenLink(t *testing.T) {
	// запускаем подменный сервер
	mockServer := NewMockServer()

	// останавливаем сервер после теста
	defer mockServer.Close()

	// настраиваем подменный HTTP-клиент, перенаправляющий запросы
	client := mockServer.Client()

	testOpts := code.Options{
		HTTPClient:  client,
		URL:         mockServer.URL,
		UserAgent:   "TestBot/1.0",
		Depth:       1,
		Concurrency: 2,
		IndentJSON:  true,
	}

	ctx := context.Background()
	jsonData, err := code.Analyze(ctx, testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// десериализуем результат для проверки структуры и контента
	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}
	// проверяем количество битых сссылок
	brokenLinks := results.Pages[0].BrokenLinks
	count := len(brokenLinks)
	if count != 1 {
		t.Errorf("expected one broken link and got it %d", count)
	}
	// порверяем url бытой ссылки
	wrongUrl := fmt.Sprintf("%s/contacts", mockServer.URL)
	if brokenLinks[0].URL != wrongUrl {
		t.Errorf("expected to receive a link - %s, but received - %s", wrongUrl, brokenLinks[0].URL)
	}
	// проверяем статус код битой ссылки
	statusNotFound := 404
	statusCodeWrongURL := brokenLinks[0].Status
	if statusCodeWrongURL != 404 {
		t.Errorf("expected to receive status code - %d, but received - %d", statusNotFound, statusCodeWrongURL)
	}
}

func TestCrawlerSEO(t *testing.T) {
	// запускаем подменный сервер
	mockServer := NewMockServer()

	// останавливаем сервер после теста
	defer mockServer.Close()

	// настраиваем подменный HTTP-клиент, перенаправляющий запросы
	client := mockServer.Client()

	testOpts := code.Options{
		HTTPClient:  client,
		URL:         mockServer.URL,
		UserAgent:   "TestBot/1.0",
		Depth:       2,
		Concurrency: 4,
		IndentJSON:  true,
	}

	ctx := context.Background()
	jsonData, err := code.Analyze(ctx, testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// десериализуем результат для проверки структуры и контента
	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}
	wantAboutSEO := code.SEO{
		HasTitle:       true,
		Title:          "General page",
		HasDescription: true,
		Description:    "Golang is the best programming language",
		HasH1:          true,
	}
	gotAboutSEO := results.Pages[1].SEO

	// проверка наличия seo показателей
	if gotAboutSEO != wantAboutSEO {
		t.Errorf("SEO indicators were expected to be %v, but we got %v", wantAboutSEO, gotAboutSEO)
	}

	wantAboutTeamSEO := code.SEO{
		HasTitle:       false,
		Title:          "",
		HasDescription: false,
		Description:    "",
		HasH1:          false,
	}
	gotAboutTeamSEO := results.Pages[2].SEO

	// проверка отсутствия seo показателей
	if gotAboutTeamSEO != wantAboutTeamSEO {
		t.Errorf("SEO indicators were expected to be %v, but we got %v", wantAboutTeamSEO, gotAboutTeamSEO)
	}
}
