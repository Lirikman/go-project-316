package main

import (
	"code"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
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

	// страница 1-го уровня глубины
	// содержит ссылку на следующий уровень
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

	// страница 2-го уровня глубины
	// содержит ссылку на следующий уровень
	mux.HandleFunc("/about/team", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `<html><body><p>Some data</p><a href="/about/team/deep">Our Team</a></body></html>`)

	})

	// страница 3-го уровня глубины
	mux.HandleFunc("/about/team/deep", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `<html><body><p>Last page</p></body></html>`)

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
	// проверяем, что в результатах нет страницы /about/team/empty
	deepUrl := fmt.Sprintf("%s/about/team/deep", mockServer.URL)
	for _, res := range results.Pages {
		if strings.Contains(res.URL, deepUrl) {
			t.Errorf("%s should not be visited due to Depth=2 restriction", deepUrl)
		}
	}
}

func TestContextCancell(t *testing.T) {
	// cервер искусственно долго отвечает, чтобы нам успеть отменить контекст
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`<html><body>Slow Page</body></html>`))
	}))
	defer server.Close()

	testOpts := code.Options{
		URL:         server.URL,
		Depth:       2,
		Retries:     1,
		Timeout:     2 * time.Second,
		RPS:         10,
		Concurrency: 1,
	}

	// cоздаем контекст с таймаутом меньше времени ответа сервера
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := code.Analyze(ctx, testOpts)

	// проверка на ошибку context deadline exceeded
	if err == nil {
		t.Fatal("Expected context deadline exceeded error, but got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Unexpected error message: %v", err)
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
		t.Errorf("Expected one broken link and got it %d", count)
	}
	// порверяем url бытой ссылки
	wrongUrl := fmt.Sprintf("%s/contacts", mockServer.URL)
	if brokenLinks[0].URL != wrongUrl {
		t.Errorf("Expected to receive a link - %s, but received - %s", wrongUrl, brokenLinks[0].URL)
	}
	// проверяем статус код битой ссылки
	statusNotFound := 404
	statusCodeWrongURL := brokenLinks[0].Status
	if statusCodeWrongURL != 404 {
		t.Errorf("Expected to receive status code - %d, but received - %d", statusNotFound, statusCodeWrongURL)
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

func TestRPSLimiter(t *testing.T) {
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
		Depth:       4,
		Timeout:     1 * time.Second,
		RPS:         2, // ограничение на 2 запроса в секунду
		Concurrency: 3,
		IndentJSON:  true,
	}
	// зафиксируем время начала анализа
	startTime := time.Now()

	ctx := context.Background()
	jsonData, err := code.Analyze(ctx, testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// рассчитаем продолжительность анализа
	duration := time.Since(startTime)

	// десериализуем результат для проверки структуры и контента
	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}

	//  проверим количество найденных страниц
	сountPage := len(results.Pages)
	if сountPage < 4 {
		t.Fatalf("Crawler processed too few pages: %d", сountPage)
	}

	// расчёт минимальной продолжительности запроса
	minDuration := time.Duration(сountPage-1) * (time.Second / time.Duration(testOpts.RPS))

	// делаем небольшую скидку - 10% на особенности планировщика и burst
	allowedDuration := minDuration - (100 * time.Millisecond)

	if duration < allowedDuration {
		t.Errorf("RPS limiter failed. Processed %d pages in %v. Expected at least %v", сountPage, duration, minDuration)
	}
}

func TestRetriesAndDelay(t *testing.T) {
	var reqCount int
	var mu sync.Mutex

	// имитируем нестабильный сервер
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		currentRequest := reqCount
		mu.Unlock()

		// первые два запроса ломаются - 500
		if currentRequest <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
			return
		}
		// третий запрос успешный - 200
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html>
							<head>
								<meta charset="UTF-8">
								<title>Test retries page</title>
								<meta name="description" content="Checking retries">
							</head>
							<body>
								<h1>Success after retries</h1>
							</body>
						</html>`))
	}))

	// останавливаем сервер после теста
	defer server.Close()

	// фиксируем время начала анализа
	startTime := time.Now()

	testOpts := code.Options{
		URL:         server.URL,
		Depth:       0,
		Retries:     2,                     // задаём 2 попытки для повтора запроса
		Delay:       50 * time.Millisecond, // задаём паузу между попытками
		Timeout:     1 * time.Second,
		RPS:         20,
		Concurrency: 1,
	}

	jsonData, err := code.Analyze(context.Background(), testOpts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// рассчитаем продолжительность анализа
	duration := time.Since(startTime)

	var results code.ReportResult
	if err := json.Unmarshal(jsonData, &results); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// проверяем на успешный парсинг страницы
	if len(results.Pages) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results.Pages))
	}
	if len(results.Pages[0].Error) > 0 {
		t.Errorf("Expected final success, but got errors: %v", results.Pages[0].Error)
	}
	wantPageSEO := code.SEO{
		HasTitle:       true,
		Title:          "Test retries page",
		HasDescription: true,
		Description:    "Checking retries",
		HasH1:          true,
	}
	gotPageSEO := results.Pages[0].SEO

	if gotPageSEO != wantPageSEO {
		t.Errorf("Unexpected body content: %v", gotPageSEO)
	}

	// проверка количества запросов
	if reqCount != 3 {
		t.Errorf("Expected exactly 3 requests to server, but got %d", reqCount)
	}

	// проверка задержки Delay
	if duration < 100*time.Millisecond {
		t.Errorf("Expected total duration to be at least 100ms due to delays, but took %v", duration)
	}
}
