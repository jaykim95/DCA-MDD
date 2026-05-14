# 📈 Go DCA Backtester

이 프로젝트는 **Yahoo Finance API**를 활용하여 특정 주식이나 ETF에 대한 **DCA(Dollar Cost Averaging, 적립식 투자)** 전략의 수익률을 시뮬레이션하는 백테스팅 도구입니다. 사용자가 입력한 주기(매주/매월)와 금액에 따라 과거 데이터를 바탕으로 투자 성과를 계산합니다.

---

## ✨ 주요 기능

* **실시간 데이터 연동**: Yahoo Finance v8 API를 통해 최신 주가 데이터를 가져옵니다.
* **DCA 시뮬레이션**: 매주 또는 매월 정해진 금액을 투자했을 때의 총 투자액, 현재 가치, 수익률을 계산합니다.
* **인증 처리**: Yahoo Finance의 쿠키 및 Crumb 토큰 인증 로직이 내장되어 있어 안정적인 데이터 수집이 가능합니다.
* **웹 인터페이스**: Go의 `html/template`을 사용한 사용자 페이지와 JSON API를 제공합니다.
* **자동 port 설정**: 서버 실행 시 빈 port를 자동으로 찾습니다.
* **자동 실행**: 서버 실행 시 기본 브라우저를 통해 자동으로 대시보드를 엽니다.

---

## 🛠 기술 스택

* **Language**: Go (Golang)
* **API**: Yahoo Finance Chart API (v8)
* **Dependencies**:
* `net/http` (서버 및 클라이언트)
* `html/template` (웹 UI)


---

## 🚀 시작하기

### 1. 사전 준비

* Go 1.16 버전 이상이 설치되어 있어야 합니다.
* 프로젝트 루트 디렉토리에 다음 파일들이 필요합니다:
* `index.html`: 결과 시각화를 위한 템플릿 파일


### 3. 종목 의존성 설치

```bash
go mod init dca-backtester
```

### 4. 실행

```bash
go run main.go
```

실행 후 자동으로 `http://localhost:xxxxx` 페이지가 브라우저에 열립니다.

---

## 📊 API 엔드포인트

### `GET /api/backtest`

백테스팅 결과를 JSON 형태로 반환합니다.

**Query Parameters:**

| 파라미터 | 설명 | 예시 |
| --- | --- | --- |
| `symbol` | 주식 티커 | `QQQ`, `AAPL` |
| `amount` | 1회 투자 금액 | `1000` |
| `startDate` | 시작 날짜 | `2017-01-03` |
| `interval` | 투자 주기 | `weekly` 또는 `monthly` |

**Response Example:**

```json
{
  "result": {
    "totalInvested": 50000,
    "totalValue": 75000.5,
    "totalUnits": 120.5,
    "profitLoss": 25000.5,
    "profitLossPct": 50.0
  },
  "history": [
    {
      "date": "2023-01-01",
      "value": 1000.0,
      "invested": 1000.0,
      "price": 150.0
    }
  ]
}

```

---

## ⚠️ 참고 사항

* 이 도구는 교육 및 참고용입니다. 실제 투자 결정 시에는 신중하게 접근하시기 바랍니다.
* Yahoo Finance API의 정책 변경에 따라 데이터 수집이 제한될 수 있습니다. 본 코드는 이를 방지하기 위한 쿠키 및 크럼브 갱신 로직을 포함하고 있습니다.
