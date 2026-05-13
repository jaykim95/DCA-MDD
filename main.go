package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

// ── Yahoo Finance API 응답 구조체 ────────────────────────────────────────────

type YFResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// ── 앱 데이터 구조체 ─────────────────────────────────────────────────────────

type BacktestResult struct {
	TotalInvested float64 `json:"totalInvested"`
	TotalValue    float64 `json:"totalValue"`
	TotalUnits    float64 `json:"totalUnits"`
	ProfitLoss    float64 `json:"profitLoss"`
	ProfitLossPct float64 `json:"profitLossPct"`
}

type HistoryPoint struct {
	Date     string  `json:"date"`
	Value    float64 `json:"value"`
	Invested float64 `json:"invested"`
	Price    float64 `json:"price"`
}

type APIResponse struct {
	Result  *BacktestResult `json:"result"`
	History []HistoryPoint  `json:"history"`
}

type PageData struct {
	Symbol    string
	Amount    float64
	StartDate string
	Interval  string
}

var (
	templatePath string
	tmpl         *template.Template
	httpClient   *http.Client
	cachedCrumb  string
)

// ── Yahoo Finance 클라이언트 초기화 ──────────────────────────────────────────

func initHTTPClient() {
	jar, _ := cookiejar.New(nil)
	httpClient = &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}
}

// Step 1: fc.yahoo.com 에서 A3 쿠키 획득
func fetchCookie() error {
	req, _ := http.NewRequest("GET", "https://fc.yahoo.com", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		// fc.yahoo.com은 connection refuse를 해도 쿠키가 set될 수 있음 — 무시
		return nil
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body)
	return nil
}

// Step 2: crumb 토큰 획득
func fetchCrumb() (string, error) {
	if cachedCrumb != "" {
		return cachedCrumb, nil
	}

	req, _ := http.NewRequest("GET", "https://query1.finance.yahoo.com/v1/test/getcrumb", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Referer", "https://finance.yahoo.com/")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("crumb 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	crumb := strings.TrimSpace(string(body))

	if crumb == "" || strings.Contains(crumb, "Too Many") || strings.Contains(crumb, "Unauthorized") {
		return "", fmt.Errorf("crumb 획득 실패 (응답: %s)", crumb)
	}

	cachedCrumb = crumb
	return crumb, nil
}

// Step 3: Yahoo Finance v8 차트 API 호출
func fetchYahooChart(symbol string, period1, period2 int64) ([]int64, []float64, error) {
	crumb, err := fetchCrumb()
	if err != nil {
		// crumb 없이 한번 시도
		crumb = ""
	}

	apiURL := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d&events=history",
		url.PathEscape(symbol), period1, period2,
	)
	if crumb != "" {
		apiURL += "&crumb=" + url.QueryEscape(crumb)
	}

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://finance.yahoo.com/")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("Yahoo API 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		// crumb 만료 → 재발급 후 재시도
		cachedCrumb = ""
		fetchCookie()
		newCrumb, err := fetchCrumb()
		if err != nil {
			return nil, nil, fmt.Errorf("crumb 재발급 실패: %w", err)
		}
		apiURL = fmt.Sprintf(
			"https://query1.finance.yahoo.com/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d&events=history&crumb=%s",
			url.PathEscape(symbol), period1, period2, url.QueryEscape(newCrumb),
		)
		req2, _ := http.NewRequest("GET", apiURL, nil)
		req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req2.Header.Set("Accept", "application/json")
		req2.Header.Set("Referer", "https://finance.yahoo.com/")
		resp2, err := httpClient.Do(req2)
		if err != nil {
			return nil, nil, fmt.Errorf("재시도 실패: %w", err)
		}
		defer resp2.Body.Close()
		resp = resp2
	}

	body, _ := io.ReadAll(resp.Body)

	var yfResp YFResponse
	if err := json.Unmarshal(body, &yfResp); err != nil {
		return nil, nil, fmt.Errorf("JSON 파싱 실패: %w (body: %.200s)", err, string(body))
	}

	if yfResp.Chart.Error != nil {
		return nil, nil, fmt.Errorf("%s: %s", yfResp.Chart.Error.Code, yfResp.Chart.Error.Description)
	}

	if len(yfResp.Chart.Result) == 0 {
		return nil, nil, fmt.Errorf("'%s' 종목 데이터를 찾을 수 없습니다", symbol)
	}

	result := yfResp.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return nil, nil, fmt.Errorf("가격 데이터가 없습니다")
	}

	return result.Timestamp, result.Indicators.Quote[0].Close, nil
}

// ── DCA 계산 ─────────────────────────────────────────────────────────────────

func calculateDCA(symbol string, amount float64, interval string, start time.Time) (*BacktestResult, []HistoryPoint, error) {
	period1 := start.Unix()
	period2 := time.Now().Unix()

	timestamps, closes, err := fetchYahooChart(symbol, period1, period2)
	if err != nil {
		return nil, nil, err
	}

	var totalUnits, totalInvested, lastPrice float64
	var history []HistoryPoint
	lastMonth := -1
	lastYearWeek := ""

	for i, ts := range timestamps {
		price := closes[i]
		if price <= 0 {
			continue
		}

		t := time.Unix(ts, 0)

		buy := false
		switch interval {
		case "monthly":
			if int(t.Month()) != lastMonth {
				buy = true
				lastMonth = int(t.Month())
			}
		case "weekly":
			year, week := t.ISOWeek()
			yw := fmt.Sprintf("%d-%d", year, week)
			if yw != lastYearWeek {
				buy = true
				lastYearWeek = yw
			}
		}

		if buy {
			totalUnits += amount / price
			totalInvested += amount
			history = append(history, HistoryPoint{
				Date:     t.Format("2006-01-02"),
				Value:    totalUnits * price,
				Invested: totalInvested,
				Price:    price,
			})
		}
		lastPrice = price
	}

	if len(history) == 0 {
		return nil, nil, fmt.Errorf("해당 기간에 매수 이력이 없습니다")
	}

	totalValue := totalUnits * lastPrice
	if lastPrice > 0 {
		history[len(history)-1].Value = totalValue
	}

	profitLoss := totalValue - totalInvested
	profitLossPct := 0.0
	if totalInvested > 0 {
		profitLossPct = (profitLoss / totalInvested) * 100
	}

	return &BacktestResult{
		TotalInvested: totalInvested,
		TotalValue:    totalValue,
		TotalUnits:    totalUnits,
		ProfitLoss:    profitLoss,
		ProfitLossPct: profitLossPct,
	}, history, nil
}

// ── HTTP 핸들러 ───────────────────────────────────────────────────────────────

func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl.Execute(w, PageData{
		Symbol:    "QQQ",
		Amount:    1000,
		StartDate: "2017-01-03",
		Interval:  "monthly",
	})
}

func handleBacktestAPI(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	amountStr := r.URL.Query().Get("amount")
	startDateStr := r.URL.Query().Get("startDate")
	interval := r.URL.Query().Get("interval")

	amount, _ := strconv.ParseFloat(amountStr, 64)
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		writeJSONError(w, "날짜 형식 오류: "+err.Error())
		return
	}

	result, history, err := calculateDCA(symbol, amount, interval, startDate)
	if err != nil {
		writeJSONError(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{Result: result, History: history})
}

func writeJSONError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ── 유틸리티 ─────────────────────────────────────────────────────────────────

func OpenURL(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		return fmt.Errorf("지원하지 않는 플랫폼")
	}
	return cmd.Start()
}

func findFile(filename string) string {
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, filename)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if ex, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(ex), filename)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	log.Fatalf("%s 파일을 찾을 수 없습니다", filename)
	return ""
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	initHTTPClient()

	// 서버 시작 전 쿠키/crumb 미리 획득
	fetchCookie()
	if _, err := fetchCrumb(); err != nil {
		log.Printf("초기 crumb 획득 실패 (요청 시 재시도): %v", err)
	}

	configPath := findFile("config.ini")
	templatePath = findFile("index.html")
	tmpl = template.Must(template.ParseFiles(templatePath))

	cfg, err := ini.Load(configPath)
	if err != nil {
		fmt.Println("config.ini 로드 실패, 기본 포트 8080 사용")
	}

	port := "8080"
	if cfg != nil {
		if p := cfg.Section("server").Key("port").String(); p != "" {
			port = p
		}
	}

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/backtest", handleBacktestAPI)

	addr := ":" + port
	serverURL := "http://localhost" + addr
	fmt.Printf("서버 시작: %s\n", serverURL)

	go func() {
		time.Sleep(800 * time.Millisecond)
		if err := OpenURL(serverURL); err != nil {
			log.Println("브라우저 열기 실패:", err)
		}
	}()

	log.Fatal(http.ListenAndServe(addr, nil))
}
