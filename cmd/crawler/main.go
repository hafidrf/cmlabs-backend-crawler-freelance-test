package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"cmlabs-backend-crawler-freelance-test/internal/crawler"
	"cmlabs-backend-crawler-freelance-test/internal/output"
)

func main() {
	cfg := parseFlags()

	urls, err := collectURLs(cfg.URLListFile, flag.Args())
	if err != nil {
		log.Fatalf("failed collecting urls: %v", err)
	}
	if len(urls) == 0 {
		log.Fatal("no URLs provided. pass URLs as args, --url-file, or stdin")
	}

	client := crawler.NewClient(crawler.Config{
		Concurrency: cfg.Concurrency,
		Timeout:     cfg.Timeout,
		UserAgent:   cfg.UserAgent,
		MinDelay:    cfg.MinDelay,
		MaxDelay:    cfg.MaxDelay,
	})

	ctx := context.Background()
	results := client.Crawl(ctx, urls)

	writer := output.NewWriter(cfg.OutputDir)
	if err := writer.Write(results); err != nil {
		log.Fatalf("failed writing output: %v", err)
	}

	out := bufio.NewWriter(os.Stdout)
	fmt.Fprintf(out, "done: crawled %d urls, outputs in %s\n", len(results), cfg.OutputDir)
	if err := out.Flush(); err != nil {
		log.Fatalf("failed flushing stdout: %v", err)
	}
}

type appConfig struct {
	Concurrency int
	Timeout     time.Duration
	UserAgent   string
	MinDelay    time.Duration
	MaxDelay    time.Duration
	OutputDir   string
	URLListFile string
}

func parseFlags() appConfig {
	cfg := appConfig{}
	flag.IntVar(&cfg.Concurrency, "concurrency", 3, "number of concurrent workers")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "per-url timeout")
	flag.StringVar(&cfg.UserAgent, "user-agent", "cmlabs-crawler-test/1.0 (+golang)", "custom user agent")
	flag.DurationVar(&cfg.MinDelay, "min-delay", 300*time.Millisecond, "minimum jitter delay before each crawl")
	flag.DurationVar(&cfg.MaxDelay, "max-delay", 1500*time.Millisecond, "maximum jitter delay before each crawl")
	flag.StringVar(&cfg.OutputDir, "out", "output", "output directory")
	flag.StringVar(&cfg.URLListFile, "url-file", "", "text file containing target URLs (1 per line)")
	flag.Parse()

	return cfg
}

func collectURLs(path string, args []string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string

	add := func(raw string) {
		u := strings.TrimSpace(raw)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}

	for _, a := range args {
		add(a)
	}

	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			add(sc.Text())
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}

	// Read stdin only when no args and no URL file are provided.
	// This avoids blocking when the process is launched in non-interactive shells.
	if len(args) == 0 && path == "" {
		stdinInfo, err := os.Stdin.Stat()
		if err != nil {
			return nil, err
		}
		if (stdinInfo.Mode() & os.ModeCharDevice) == 0 {
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				add(sc.Text())
			}
			if err := sc.Err(); err != nil {
				return nil, err
			}
		}
	}

	for _, u := range out {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return nil, errors.New("all URLs must start with http:// or https://")
		}
	}

	return out, nil
}
