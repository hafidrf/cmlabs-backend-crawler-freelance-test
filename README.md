# cmlabs Backend Crawler Test (Go)

## Struktur

- `cmd/crawler/main.go` : entry point CLI
- `internal/crawler/crawler.go` : proses crawling (chromedp)
- `internal/output/writer.go` : simpan file HTML dan `manifest.json`
- `targets.txt` : daftar target URL
- `output/` : hasil crawling

## Requirements

- Go 1.22+
- Google Chrome / Chromium

## Cara Menjalankan

### 1. Install dependensi

```bash
go mod tidy
```

### 2. Jalankan crawler dari file target

```bash
go run ./cmd/crawler --url-file targets.txt --out output --concurrency 2 --timeout 120s
```

### 3. Alternatif (URL langsung)

```bash
go run ./cmd/crawler https://cmlabs.co https://sequence.day https://go.dev --out output
```

## Output

Folder `output/` berisi:

- file HTML per website
- `manifest.json` (URL, status, durasi, judul, nama file)

## Target yang digunakan

- `https://cmlabs.co`
- `https://sequence.day`
- `https://go.dev`
