# cmlabs Backend Crawler Test (Go)

Project ini adalah crawler website berbasis Go untuk kebutuhan technical test.  
Fokusnya: bisa crawl website modern (termasuk SPA/PWA) dan menyimpan hasil akhir HTML ke file.

## Kenapa ini beda

- CLI bergaya unix: bisa input URL dari argumen, file, atau piped stdin.
- Support rendering JavaScript memakai headless Chrome (`chromedp`), bukan sekadar HTTP fetch.
- Concurrent worker + jitter delay untuk crawling lebih natural dan stabil.
- Output terstruktur: file HTML per website + `manifest.json`.

## Struktur

- `cmd/crawler/main.go` - entry point CLI
- `internal/crawler` - engine crawling + browser automation
- `internal/output` - writer untuk HTML dan manifest
- `targets.txt` - daftar target sesuai requirement

## Requirement

- Go 1.22+
- Google Chrome atau Chromium terpasang di OS

## Menjalankan

### 1) Install dependency

```bash
go mod tidy
```

### 2) Crawl dari file target

```bash
go run ./cmd/crawler --url-file targets.txt --out output --concurrency 3
```

### 3) Alternatif: unix pipe

```bash
cat targets.txt | go run ./cmd/crawler --out output
```

### 4) Alternatif: URL langsung

```bash
go run ./cmd/crawler https://cmlabs.co https://sequence.day https://go.dev
```

## Hasil

Folder `output/` akan berisi:

- `*.html` => snapshot hasil render final dari masing-masing website
- `manifest.json` => metadata crawl (status, durasi, judul halaman, path file)

Setelah selesai, CLI akan mencetak baris `done: crawled ...` (stdout di-flush agar langsung terlihat).

## Cara mengecek hasil kerja

1. **Pastikan crawl sukses** — di terminal harus muncul `done: crawled N urls, outputs in output` (atau folder `--out` yang kamu pakai).
2. **Buka `output/manifest.json`** — pastikan tiap URL punya `"status": "success"`. Kalau ada `"error"`, baca pesannya (timeout, blokir, dll.).
3. **Buka file `.html` di browser** — double-click file di Explorer atau drag ke Chrome/Edge. Isinya harus halaman yang sudah di-render (bukan halaman kosong/error), judul tab biasanya cocok dengan `"title"` di manifest.
4. **(Opsional) Bandingkan dengan situs asli** — buka URL asli di tab lain; struktur konten dan teks utama seharusnya mirip (waktu crawl beda bisa sedikit beda konten dinamis).
5. **Untuk pengumpulan test** — commit folder `output/` ke repo publik jika recruiter minta hasil HTML ikut di-upload (sesuai soal).

## Catatan etika

- Gunakan concurrency secara wajar.
- Hormati Terms of Service tiap website.
- Hindari crawling agresif.
