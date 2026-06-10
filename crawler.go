package code

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/html"
)

// параметры для выполнения запроса
type Options struct {
	Client      *http.Client
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
}

// структура отчёта одной страницы
type Page struct {
	URL        string `json:"url"`
	Depth      int    `json:"depth"`
	HTTPStatus int    `json:"http_status"`
	Status     string `json:"status"`
	Error      error  `json:"error,omitempty"`
}

// структура финального JSON-отчета с заданной глубиной
type ReportResult struct {
	RootURL     string `json:"root_url"`
	Depth       int    `json:"depth"`
	GeneratedAt string `json:"generated_at"`
	Pages       []Page `json:"pages"`
}

// функция рекурсивного обхода сайта для поиска ссылок
func findLinks(n *html.Node, rootURL string, depth int) []string {
	var links []string
	// создаём счётчик ссылок
	counterLink := 0
	var f func(*html.Node)
	f = func(n *html.Node) {
		// если показатель счётчика больше параметра depth
		if counterLink > depth {
			return
		}
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
					counterLink++
					// парсим только страницы текущего домена, не выходим за его пределы
					// if strings.HasPrefix(absoluteURL, rootURL) {
					// 	links = append(links, absoluteURL)
					// }
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

// перменная для хранения посещённых URL
var linksVisited = make(map[string]bool)

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
	if linksVisited[url] {
		return Page{}
	}
	// добавляем запись о посещении
	linksVisited[url] = true
	// если http-сlient не задан, то создаем стандартный
	client := opts.Client
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
		}
	}
	// создаём запрос
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		report.Error = fmt.Errorf("error creating request: %v", err)
		return report
	}
	// если user_agent не задан
	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}
	// выполнение запроса
	resp, err := client.Do(req)
	if err != nil {
		report.HTTPStatus = resp.StatusCode
		report.Status = http.StatusText(resp.StatusCode)
		report.Error = fmt.Errorf("request execution error: %v", err)
		return report
	}
	// освобождение ресурсов после запроса
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("failed to close response body: %v", err)
		}
	}()
	// формирование итогового отчета о странице
	report = Page{
		URL:        url,
		Depth:      opts.Depth,
		HTTPStatus: resp.StatusCode,
		Status:     http.StatusText(resp.StatusCode),
		Error:      fmt.Errorf("page parsing error: %v", err),
	}
	return report
}

// функция анализа сайта с заданными параметрами
func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	// переменная для хранения итогового отчёта
	var reportBytes []byte
	// // добавляем запись о посещении
	// linksVisited[rootUrl] = true
	// создание запроса
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	// если http client не задан создаём стандтный
	client := opts.Client
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
		}
	}
	// если user_agent не задан
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
	// находим все ссылки на странице c учётом парамента depth
	urls := findLinks(doc, opts.URL, opts.Depth)
	fmt.Println("Найденные ссылки: ", urls)
	// задаём счётчик для параметра depth
	counterPage := 0
	// проходим по найденным ссылкам
	for _, u := range urls {
		// задаём условия ограничения парсинга
		if counterPage > opts.Depth {
			break
		}
		// задаём параметры поиска страницы
		optsPage := opts
		optsPage.URL = u
		optsPage.Depth = counterPage
		// запускаем парсинг страницы
		parsPage := ParsUrl(ctx, optsPage)
		// добавляем информацию в отчёт по странице
		emptyPage := Page{}
		if parsPage != emptyPage {
			report.Pages = append(report.Pages, parsPage)
		}
		// увеличиваем показания счётика на 1
		counterPage++
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
	fmt.Println("Посещённые ссылки: ", linksVisited)
	return reportBytes, nil
}
