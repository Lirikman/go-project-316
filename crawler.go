package code

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// параметры для выполнения запроса
type Options struct {
	HTTPClient  *http.Client
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
}

// структуря для SEO-показателей
type SEO struct {
	HasTitle       bool   `json:"has_title"`
	Title          string `json:"title"`
	HasDescription bool   `json:"has_description"`
	Description    string `json:"description"`
	HasH1          bool   `json:"has_h1"`
}

// структура отчёта одной страницы
type Page struct {
	URL          string    `json:"url"`
	Depth        int       `json:"depth"`
	HTTPStatus   int       `json:"http_status"`
	Status       string    `json:"status"`
	BrokenLinks  []BadLink `json:"broken_links,omitempty"`
	SEO          SEO       `json:"seo"`
	DiscoveredAT string    `json:"discovered_at"`
}

// структура 'битых' ссылок
type BadLink struct {
	URL    string `json:"url"`
	Status int    `json:"status_code,omitempty"`
	Error  string `json:"error,omitempty"`
}

// структура финального JSON-отчета с заданной глубиной
type ReportResult struct {
	RootURL     string `json:"root_url"`
	Depth       int    `json:"depth"`
	GeneratedAt string `json:"generated_at"`
	Pages       []Page `json:"pages"`
}

// функция поиска ссылок на сайте
func findLinks(n *html.Node, rootURL string) []string {
	var links []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		// находим узел-элемент и тег <a>
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				// находим атрибут href - ссылку
				if a.Key == "href" {
					// нормализация ссылки
					u, err := url.Parse(a.Val)
					if err != nil {
						continue
					}
					base, _ := url.Parse(rootURL)
					absoluteURL := base.ResolveReference(u).String()
					links = append(links, absoluteURL)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	// возвращаем найденные ссылки
	return links
}

// функция проверки ссылки на 'битость'
func CheckLink(urlStr string) BadLink {
	wrongLink := BadLink{}
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	// сначала выполним HEAD запрос
	req, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		return BadLink{}
	}
	// выполняем запрос
	respHead, err := client.Do(req)
	if err != nil {
		wrongLink.URL = urlStr
		wrongLink.Error = fmt.Sprintf("%v", err)
		return wrongLink
	}
	defer respHead.Body.Close()
	// если сервер запретил HEAD, пробуем GET
	if respHead.StatusCode == http.StatusMethodNotAllowed || respHead.StatusCode == http.StatusForbidden {
		reqGet, err := http.NewRequest("GET", urlStr, nil)
		if err != nil {
			return BadLink{}
		}
		// выполняем запрос
		respGet, err := client.Do(reqGet)
		if err != nil {
			wrongLink.URL = urlStr
			wrongLink.Error = fmt.Sprintf("Get: '%s': %v", urlStr, err)
			return wrongLink
		}
		defer respGet.Body.Close()
		if respGet.StatusCode >= 400 {
			wrongLink.URL = urlStr
			wrongLink.Status = respGet.StatusCode
		}
		return wrongLink
	}
	if respHead.StatusCode >= 400 {
		wrongLink.URL = urlStr
		wrongLink.Status = respHead.StatusCode
	}
	return wrongLink
}

// структура для хранения истории посещения сайтов
type History struct {
	urls    []string
	visited map[string]struct{}
}

// функция создания нового экземпляра для сохранения истории
func NewHistory() *History {
	return &History{
		urls:    make([]string, 0),
		visited: make(map[string]struct{}),
	}
}

// функция добавления новой записи о посещении сайта
func (h *History) Add(url string) {
	if _, exists := h.visited[url]; !exists {
		h.visited[url] = struct{}{}
		h.urls = append(h.urls, url)
	}
}

// создаём структуру для хранения посещённых URL, и сохранения порядка посещений
var linksVisited = NewHistory()

// функция парсинга одной страницы с заданными параметрами
func ParsUrl(ctx context.Context, opts Options) Page {
	// подготавливаем структуру для отчёта
	report := Page{
		URL:   opts.URL,
		Depth: opts.Depth,
	}
	// получаем url для парсинга
	url := opts.URL
	// проверяем запись о посещении для данного url
	if _, exists := linksVisited.visited[url]; exists {
		return Page{}
	}
	// добавляем запись о посещении
	linksVisited.Add(url)
	// если http-сlient не задан, то создаем стандартный
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
		}
	}
	// создаём запрос
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Page{}
	}
	// если user_agent не задан
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}
	// выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		return Page{}
	}
	// освобождение ресурсов после запроса
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Fatalf("failed to close response body: %v", err)
		}
	}()
	// проверка статус кода
	if resp.StatusCode != http.StatusOK {
		return Page{}
	}
	// загружаем html в goquery
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatalf("loading error in goquery: %v", err)
	}
	// создаём структуру для показателей SEO
	var pageSE0 SEO
	// парсим тег title
	title := doc.Find("title").Text()
	title = strings.TrimSpace(title)
	if title != "" {
		pageSE0.HasTitle = true
		pageSE0.Title = title
	}
	// ищем тег meta с name = "description", и берем атрибут content
	description, exists := doc.Find("meta[name='description']").Attr("content")
	if exists {
		description = strings.TrimSpace(description)
	}
	if description != "" {
		pageSE0.HasDescription = true
		pageSE0.Description = description
	}
	// парсим тег h1
	h1 := doc.Find("h1").Text()
	if h1 != "" {
		pageSE0.HasH1 = true
	}
	// парсинг HTML
	docNew, err := html.Parse(resp.Body)
	if err != nil {
		return Page{}
	}
	// находим все ссылки на странице
	var links []string
	links = findLinks(docNew, url)
	// Проверяем каждую ссылку
	for _, link := range links {
		// пропуск пустых ссылок или якорей (#)
		if link == "" || strings.HasPrefix(link, "#") {
			continue
		}
		res := CheckLink(link)
		emptyLink := BadLink{}
		if res != emptyLink {
			report.BrokenLinks = append(report.BrokenLinks, res)
		}
	}
	// формирование итогового отчета о странице
	currentTime := time.Now().Format(time.RFC3339)
	report = Page{
		URL:          url,
		Depth:        opts.Depth,
		HTTPStatus:   resp.StatusCode,
		Status:       http.StatusText(resp.StatusCode),
		SEO:          pageSE0,
		DiscoveredAT: currentTime,
	}
	return report
}

// функция анализа сайта с заданными параметрами
func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	// переменная для хранения итогового отчёта
	var reportBytes []byte
	// добавляем запись о посещении
	linksVisited.Add(opts.URL)
	// создание запроса
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	// если http client не задан задаём стандартный
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
		}
	}
	// если user_agent не задан задаём стандартный
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}
	// выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request execution error: %w", err)
	}
	// освобождение ресурсов после запроса
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()
	// проверка статус кода
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid status code: %d", resp.StatusCode)
	}
	// парсинг HTML
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("page parsing error:%v", err)
	}
	// формирование отчета
	currentTime := time.Now().Format(time.RFC3339)
	report := ReportResult{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: currentTime,
		Pages:       []Page{},
	}
	// задаём текущее показание depth
	currentDepth := opts.Depth - 1
	// проходим по найденным ссылкам в глубину
	for i := currentDepth; i >= 0; i-- {
		// находим все ссылки на странице
		urls := findLinks(doc, linksVisited.urls[len(linksVisited.urls)-1])
		fmt.Println("Найденные ссылки: ", urls)
		// получаем случайный индекс на ссылку
		randIdx := rand.Intn(len(urls))
		// берём элемент по индексу
		randUrl := urls[randIdx]
		// задаём параметры поиска страницы
		optsPage := opts
		optsPage.URL = randUrl
		optsPage.Depth = currentDepth
		// запускаем парсинг страницы
		parsPage := ParsUrl(ctx, optsPage)
		// добавляем информацию в отчёт по странице
		emptyPage := Page{}
		// если отчёт не пустой то добавлем к основному отчёту
		if !reflect.DeepEqual(parsPage, emptyPage) {
			report.Pages = append(report.Pages, parsPage)
		}
		// уменьшаем значение показателья depth на 1
		currentDepth--
	}
	// cериалиализация отчета в JSON с заданным параметром indent-json
	var serialErr error
	// если indent-json true
	if opts.IndentJSON {
		reportBytes, serialErr = json.MarshalIndent(report, "", "  ")
		// если indent-json false
	} else {
		reportBytes, serialErr = json.Marshal(report)
	}
	// возвращаем результаты запроса
	if serialErr != nil {
		return nil, fmt.Errorf("report serialization error: %w", serialErr)
	}
	fmt.Println("Посещённые ссылки: ", linksVisited.visited)
	return reportBytes, nil
}
