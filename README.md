## Hexlet go crawler

### Hexlet tests and linter status:
[![Actions Status](https://github.com/Lirikman/go-project-316/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/Lirikman/go-project-316/actions)

### Description

Analyze a website structure

USAGE:
   hexlet-go-crawler [global options] <url>

GLOBAL OPTIONS:

   --depth int          crawl depth (default: 10)
   
   --retries int        number of retries for failed requests (default: 1)
   
   --delay duration     delay between requests (example: 200ms, 1s) (default: 0s)
   
   --timeout duration   per-request timeout (default: 15s)
   
   --rps int            limit requests per second (overrides delay) (default: 0)
   
   --user-agent string  custom user agent
   
   --workers int        number of concurrent workers (default: 4)
   
   --indent-json        formatting output in json
   
   --help, -h           show help

### Requirements

* Go 1.26
* Make
* urfave/cli v3

### Run build

```bash
make build
```

### Run golangci-lint 

```bash
make lint
```

### Run tests

```bash
make test
```

### Running a query

```bash
make run URL="https://example.com"
```

### Example of work

```bash
./crawler --depth=10 https://example.com
```
#### Answer:
```json
{
  "root_url": "https://example.com",
  "depth": 10,
  "generated_at": "2026-06-25T22:28:46+07:00",
  "pages": [
    {
      "url": "https://example.com",
      "depth": 0,
      "http_status": 200,
      "status": "ok",
      "seo": {
        "has_title": true,
        "title": "Example Domain",
        "has_description": false,
        "description": "",
        "has_h1": true
      },
      "broken_links": [],
      "assets": [
        {
          "url": "data:,",
          "type": "other",
          "status_code": 0,
          "size_bytes": 0,
          "error": "Get \"data:,\": unsupported protocol scheme \"data\""
        }
      ],
      "discovered_at": "2026-06-25T22:28:46+07:00"
    }
  ]
}
```
