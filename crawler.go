package code

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/time/rate"
)

// параметры для выполнения запроса
type Options struct {
	HTTPClient  *http.Client
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	RPS         int
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
}

// структуря для SEO-показателей страницы
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
	LinksFound   []string  `json:"-"`
	BrokenLinks  []BadLink `json:"broken_links,omitempty"`
	SEO          SEO       `json:"seo"`
	DiscoveredAT string    `json:"discovered_at"`
	Error        string    `json:"error,omitempty"`
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

// структура задачи
type Task struct {
	url   string
	depth int
}

// функция преобразования относительной ссылки в абсолютную
func resolveURL(base *url.URL, href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(u).String()
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

// функция фильтрации списка сайтов по домену
func filterDomain(targetDomain string, links []string) []string {
	var matchedURLs []string
	for _, u := range links {
		parsedURL, err := url.Parse(u)
		if err != nil {
			log.Printf("parsing error %s: %v\n", u, err)
			continue
		}
		// убедимся, что URL содержит хост
		if parsedURL.Host == "" {
			continue
		}
		// проверяем, совпадает ли домен или является ли поддоменом
		host := strings.ToLower(parsedURL.Host)
		if host == targetDomain || strings.Contains(host, targetDomain) {
			matchedURLs = append(matchedURLs, u)
		}
	}
	return matchedURLs
}

// функция анализа сайта с заданными параметрами
func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	// если http client не задан задаём стандартный
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{
			Timeout: opts.Timeout,
		}
	}

	// создаём буферизованные каналы для парсинга сайтов и сбора результатов
	tasksChan := make(chan Task, 10000)
	resultsChan := make(chan Page, 10000)

	// создаём счётчик ожидания завершения всех горутин
	var wg sync.WaitGroup

	// ограничиваем доступ к разделяемым данным и предотвращаем 'состояние гонки'
	var mu sync.Mutex

	// создаём и инициализируем карту для сохранения посещённых сайтов
	visited := make(map[string]bool)

	// настраиваем глобальный лимитер частоты запросов RPS
	var limReq *rate.Limiter
	if opts.RPS > 0 {
		limReq = rate.NewLimiter(rate.Limit(opts.RPS), opts.RPS)
	}

	// функция воркера
	worker := func() {
		// запуск цикла
		for {
			select {
			// ожидание отмены контекста
			case <-ctx.Done():
				return
			// ожидание передачи задачи в канал
			case t, ok := <-tasksChan:
				if !ok {
					return
				}
				// ограничение частоты запросов
				if limReq != nil {
					// если задан RPS, ждем разрешения от глобального лимитера
					if err := limReq.Wait(ctx); err != nil {
						wg.Done()
						return
					}
				} else if opts.Delay > 0 {
					// если RPS не задан, используем задержку Delay
					select {
					case <-time.After(opts.Delay):
					case <-ctx.Done():
						wg.Done()
						return
					}
				}

				// парсинг страницы по параметрам текущего задания
				pageRes := ParsPage(ctx, t.url, t.depth, opts)
				// передаём результаты парсинга страницы в канал resultsChan
				resultsChan <- pageRes

				// проверяем параметр depth b добавляем новые ссылки в очередь
				if t.depth < opts.Depth && pageRes.Error == "" {
					// фильтруем полученные ссылки по домену
					u, _ := url.Parse(opts.URL)
					hostName := u.Hostname()
					domainUrls := filterDomain(hostName, pageRes.LinksFound)
					// fmt.Println("Отфильтрованные страницы: ", domainUrls)
					mu.Lock()
					for _, link := range domainUrls {
						// проверяем запись о посещении для данного url
						if !visited[link] {
							// добавляем запись о посещении
							visited[link] = true
							// увеличиваем счётчик задач
							wg.Add(1)
							// отправляем новое задание в отдельной горутине
							go func(l string, d int) {
								tasksChan <- Task{url: l, depth: d}
							}(link, t.depth+1)
							// прерываем цикл после первой успешной обработки
							break
						}
					}
					mu.Unlock()
				}
				wg.Done()
			}
		}
	}
	// запуск пула воркеров
	for i := 0; i < opts.Concurrency; i++ {
		go worker()
	}

	// добавление стартового URL в список посещенных сайтов
	visited[opts.URL] = true
	wg.Add(1)
	tasksChan <- Task{url: opts.URL, depth: 0}

	// отслеживание завершения всех горутин
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(tasksChan)
		close(resultsChan)
		close(done)
	}()

	// ожидание завершения или отмены контекста
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}

	// сбор результатов и формирование итогового отчёта
	var reportBytes []byte
	currentTime := time.Now().Format(time.RFC3339)
	report := ReportResult{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: currentTime,
		Pages:       []Page{},
	}

	// получение данных из канала resultsChan
	for res := range resultsChan {
		report.Pages = append(report.Pages, res)
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
	if serialErr != nil {
		return nil, fmt.Errorf("report serialization error: %w", serialErr)
	}
	// возвращаем итоговый отчёт анализа сайта
	return reportBytes, nil
}

// функция парсинга одной страницы
func ParsPage(ctx context.Context, targetURL string, currentDepth int, opts Options) Page {
	// создаём переменную для отчёта
	var report Page
	// создаём переменные ответа запроса и ошибки
	var resp *http.Response
	var err error

	// запускаем цикл повторных попыток - параметр retries
	for i := 0; i <= opts.Retries; i++ {
		// delay используем для паузы между попытками
		if i > 0 && opts.Delay > 0 {
			select {
			case <-ctx.Done():
				report.Error = ctx.Err().Error()
				return report
			case <-time.After(opts.Delay):
			}
		}
		// создание запроса
		req, reqErr := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
		if reqErr != nil {
			err = reqErr
			continue
		}
		// если user-agent на задан
		if opts.UserAgent == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		}
		// если код ответа успешный, то выходим из цикла
		resp, err = opts.HTTPClient.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			break
		}
		// если тело ответа не пустое, то освобождаем ресурсы
		if resp != nil {
			resp.Body.Close()
		}
	}
	// если после всех попыток ошибка, то сохраняем её в отчёте
	if err != nil {
		report.Error = err.Error()
		return report
	}
	// освобождаем ресурсы
	defer resp.Body.Close()

	// загружаем полученный html в goquery для поиска тегов
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		report.Error = fmt.Sprintf("loading error in goquery: %v", err)
	}
	// создаём структуру для показателей SEO
	var pageSE0 SEO
	// парсим тег <title>
	title := doc.Find("title").Text()
	title = strings.TrimSpace(title)
	if title != "" {
		pageSE0.HasTitle = true
		pageSE0.Title = title
	}
	// ищем тег <meta> с name = "description", и берем атрибут content
	description, exists := doc.Find("meta[name='description']").Attr("content")
	if exists {
		description = strings.TrimSpace(description)
	}
	if description != "" {
		pageSE0.HasDescription = true
		pageSE0.Description = description
	}
	// парсим тег <h1>
	h1 := doc.Find("h1").Text()
	if h1 != "" {
		pageSE0.HasH1 = true
	}
	// находим все ссылки на странице
	var links []string
	// парсим тег <a>
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		// получаем значение атрибута href
		href, exists := s.Attr("href")
		if exists && href != "" {
			// преобразуем относительные ссылки в абсолютные
			baseURL, _ := url.Parse(targetURL)
			absURL := resolveURL(baseURL, href)
			links = append(links, absURL)
		}
	})
	// проверяем каждую ссылку
	var brokenLinks []BadLink
	// создадим список битых ссылок
	var wrongLinks []string
	for _, link := range links {
		// пропуск пустых ссылок или якорей (#)
		if link == "" || strings.HasPrefix(link, "#") {
			continue
		}
		res := CheckLink(link)
		emptyLink := BadLink{}
		if res != emptyLink {
			brokenLinks = append(brokenLinks, res)
			wrongLinks = append(wrongLinks, res.URL)
		}
	}
	// сохраняем только не битые ссылки
	for _, link := range links {
		if !slices.Contains(wrongLinks, link) {
			report.LinksFound = append(report.LinksFound, link)
		}
	}
	// формирование итогового отчета о странице
	currentTime := time.Now().Format(time.RFC3339)
	report = Page{
		URL:          targetURL,
		Depth:        currentDepth,
		HTTPStatus:   resp.StatusCode,
		Status:       http.StatusText(resp.StatusCode),
		SEO:          pageSE0,
		LinksFound:   links,
		BrokenLinks:  brokenLinks,
		DiscoveredAT: currentTime,
	}
	// возвращаем отчёт о странице
	return report
}
