package main

import (
	"code"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/urfave/cli/v3"
)

func main() {
	var url string
	// cтруктура для валидации url
	type UrlValidate struct {
		SiteURL string `validate:"required,url"`
	}
	cmd := &cli.Command{
		Name:      "hexlet-go-crawler",
		Usage:     "analyze a website structure",
		Commands:  []*cli.Command{},
		ArgsUsage: "<url>",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "url",
				Destination: &url,
			},
		},
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "depth",
				Value: 10,
				Usage: "crawl depth",
			},
			&cli.IntFlag{
				Name:  "retries",
				Value: 1,
				Usage: "number of retries for failed requests",
			},
			&cli.DurationFlag{
				Name:  "delay",
				Value: 0 * time.Second,
				Usage: "delay between requests (example: 200ms, 1s)",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Value: 15 * time.Second,
				Usage: "per-request timeout",
			},
			&cli.IntFlag{
				Name:  "rps",
				Value: 0,
				Usage: "limit requests per second (overrides delay)",
			},
			&cli.StringFlag{
				Name:  "user-agent",
				Usage: "custom user agent",
			},
			&cli.IntFlag{
				Name:  "workers",
				Value: 4,
				Usage: "number of concurrent workers",
			},
			&cli.BoolFlag{
				Name:  "indent-json",
				Value: true,
				Usage: "formatting output in json",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// проверка на ввод url
			if url == "" {
				return fmt.Errorf("the URL cannot be empty")
			}
			// валидация аргумента url
			validate := validator.New()
			resource := &UrlValidate{SiteURL: url}
			err := validate.Struct(resource)
			if err != nil {
				for _, err := range err.(validator.ValidationErrors) {
					fmt.Printf("Поле: %s, Ошибка: тег '%s' не выполнен\n", err.Field(), err.Tag())
				}
			}
			// получаем параметры запроса из флагов
			opt := code.Options{
				URL:         url,
				Depth:       cmd.Int("depth"),
				Retries:     cmd.Int("retries"),
				Delay:       cmd.Duration("delay"),
				Timeout:     cmd.Duration("timeout"),
				UserAgent:   cmd.String("user-agent"),
				Concurrency: cmd.Int("workers"),
				IndentJSON:  cmd.Bool("indent-json"),
			}
			// выполняем запрос
			res, err := code.Analyze(ctx, opt)
			// выводим результаты
			if err == nil {
				fmt.Println(string(res))
			}
			return err
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
