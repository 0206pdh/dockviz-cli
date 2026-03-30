# dockviz-cli — 기술 심층 분석

> 언어 선택, 라이브러리 결정, 구현 흐름, 그리고 그 과정에서 마주친 기술적 문제들

---

## 1. 왜 Go인가

### 단일 바이너리 배포

Go는 런타임, 표준 라이브러리, 의존성을 하나의 실행 파일로 컴파일한다. `dockviz`를 서버에 배포하는 데 필요한 것은 바이너리 파일 하나뿐이다. Python이라면 `pip install`, Node라면 `node_modules` — 그런 것이 없다. SSH로만 접근하는 서버에서 `curl` 한 줄로 설치되는 툴을 만드는 데 이 특성이 핵심이다.

### 크로스 컴파일

환경변수 두 개로 6개 플랫폼 바이너리를 뽑는다:

```bash
GOOS=linux   GOARCH=amd64  go build -o dockviz-linux-amd64
GOOS=darwin  GOARCH=arm64  go build -o dockviz-darwin-arm64
GOOS=windows GOARCH=amd64  go build -o dockviz-windows-amd64.exe
```

GitHub Actions에서 matrix 전략으로 이를 자동화한다. C/C++에서의 크로스 컴파일과 비교하면 이 단순함은 상당한 이점이다.

### goroutine과 채널

TUI는 본질적으로 동시성 문제다. 화면 렌더링, 2초마다 Docker 데이터 fetch, 컨테이너 로그 실시간 수신, Docker 데몬 이벤트 스트리밍이 동시에 돌아야 한다. Go의 goroutine은 OS 스레드보다 훨씬 가볍고(스택 초기 2KB), 채널로 goroutine 간 통신을 명시적으로 제어할 수 있다. 이 프로젝트의 로그 스트리밍과 이벤트 스트리밍 구조가 이 특성을 직접 활용한다.

### Docker SDK

Docker는 Go로 작성되었고, Docker SDK for Go는 Docker가 직접 관리하는 공식 클라이언트다. HTTP API를 직접 호출하는 것보다 타입 안정성이 보장되고, API 버전 협상(`client.WithAPIVersionNegotiation()`)도 자동으로 처리된다.

---

## 2. 핵심 라이브러리 선택 이유

### Bubble Tea — The Elm Architecture for TUI

Bubble Tea는 Elm 언어의 아키텍처(TEA)를 Go로 구현한다:

```
Model  — 앱의 전체 상태 (구조체 하나)
Update — 메시지를 받아 새 Model을 반환하는 순수 함수
View   — Model을 받아 문자열을 반환하는 순수 함수
```

상태가 단일 `Model` 구조체에만 존재하고, 상태 변경은 오직 `Update` 함수를 통해서만 일어난다. 상태 변경 경로가 하나뿐이기 때문에 버그 추적이 쉽다. "어디서 상태가 바뀌었나?"를 찾을 필요가 없다 — 항상 `Update`다.

**Commands 패턴**: `Update`는 I/O를 직접 수행하지 않고 "이 작업을 나중에 실행해라"는 `Cmd`를 반환한다. Bubble Tea 런타임이 goroutine에서 이를 실행하고 결과를 다시 `Update`로 전달한다. 덕분에 `Update`는 순수 함수로 유지된다:

```go
// tickMsg가 오면 데이터 fetch를 예약하고, 다음 tick도 예약한다.
case tickMsg:
    return m, tea.Batch(fetchDataCmd(m.docker), tickCmd())
```

### Lip Gloss — 스타일 선언

터미널 색상을 `fmt.Sprintf("\033[32m%s\033[0m", text)` 방식으로 직접 ANSI 코드를 박으면 코드가 지저분해지고 유지보수가 어렵다. Lip Gloss는 CSS와 유사한 선언적 스타일을 제공한다:

```go
TitleStyle = lipgloss.NewStyle().
    Bold(true).
    Foreground(ColorBlue).
    Padding(0, 1)
```

색상 팔레트를 `styles.go` 한 파일에 모아두어 테마 변경이 하나의 파일 수정으로 끝난다.

### Cobra — CLI 구조

`dockviz pull <image>` 서브커맨드가 있다. Cobra는 서브커맨드, 플래그 파싱, 자동 help 생성을 제공한다(kubectl, Hugo, GitHub CLI 등이 Cobra 기반이다). `--demo` 플래그와 `pull` 서브커맨드를 자연스럽게 수용하면서 향후 확장에도 대응 가능한 구조를 제공한다.

---

## 3. 아키텍처: 인터페이스 분리

프로젝트의 가장 중요한 설계 결정은 `DockerClient` 인터페이스다:

```go
type DockerClient interface {
    ListContainers() ([]ContainerInfo, error)
    ListNetworks() ([]NetworkInfo, error)
    ListImages() ([]ImageInfo, error)
    FetchStats(id string) (cpu float64, memMB float64, err error)
    StartContainer(id string) error
    StopContainer(id string) error
    RemoveContainer(id string) error
    RemoveImage(id string) error
    StreamLogs(ctx context.Context, id string) <-chan LogLine
    StreamEvents(ctx context.Context) <-chan EventInfo
    Close()
}
```

TUI 코드(`internal/tui/`)는 이 인터페이스에만 의존한다. 실제 Docker 데몬에 연결하는 `Client`와 가짜 데이터를 생성하는 `DemoClient` 모두 이를 구현한다. 덕분에 진입점에서 구현체만 교체하면 된다:

```go
// cmd/root.go
if demo {
    dc = docker.NewDemoClient()
} else {
    dc, err = docker.NewClient()
}
tui.Start(dc, version)
```

---

## 4. 전체 구현 흐름

### 4-1. 2초 주기 데이터 갱신

Bubble Tea의 `tea.Tick`으로 자동 갱신을 구현한다:

```go
func tickCmd() tea.Cmd {
    return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

`tickMsg`가 오면 `fetchDataCmd`를 실행하고 동시에 다음 `tickCmd`를 예약한다. 데이터 fetch는 goroutine에서 비동기로 실행되므로 메인 루프(UI 렌더링)를 블록하지 않는다.

### 4-2. CPU 사용량 계산

Docker Stats API는 절대 나노초 값을 준다. 퍼센트를 구하려면 이전 snapshot과의 delta 공식이 필요하다:

```go
func calcCPUPercent(stats container.StatsResponse) float64 {
    cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
        float64(stats.PreCPUStats.CPUUsage.TotalUsage)
    sysDelta := float64(stats.CPUStats.SystemUsage) -
        float64(stats.PreCPUStats.SystemUsage)
    numCPU := float64(stats.CPUStats.OnlineCPUs)
    if sysDelta == 0 {
        return 0
    }
    return (cpuDelta / sysDelta) * numCPU * 100.0
}
```

### 4-3. CPU Sparkline

컨테이너별 최근 10개 CPU 값을 `map[string][]float64` 히스토리에 쌓는다:

```go
h = append(h, c.CPUPerc)
if len(h) > 10 {
    h = h[len(h)-10:]
}
```

`Sparkline()` 함수는 값들을 `▁▂▃▄▅▆▇█` 8단계 블록 문자로 변환한다. 최댓값 기준으로 정규화하기 때문에 낮은 CPU에서도 상대적 변화를 볼 수 있다. 절대값 기준이면 미세한 변화가 보이지 않는다.

### 4-4. ANSI 컬럼 정렬 문제

가장 까다로운 버그 중 하나다. `fmt.Sprintf("%-12s", value)`로 컬럼을 정렬할 때, ANSI 색상 코드가 포함된 문자열은 `len()`이 시각적 너비보다 훨씬 크다. `%s` 포맷터가 잘못된 패딩을 계산해 컬럼이 틀어진다.

해결 방법: **색상 입히기 전에 먼저 패딩을 적용**한다:

```go
// 틀린 방식
statusStr := fmt.Sprintf("%-12s", ui.StatusStyle(c.Status).Render("● running"))

// 올바른 방식: 평문 패딩 먼저, 색상 나중
statusText := fmt.Sprintf("%-12s", "● "+c.Status) // 12자 패딩
statusStr  := ui.StatusStyle(c.Status).Render(statusText) // 그 다음 색상
```

Sparkline도 같은 원칙이다. `ui.Sparkline()`이 항상 10 rune 너비 문자열을 반환하고, 그 다음에 색상을 적용한다.

### 4-5. 실시간 로그 스트리밍

채널과 Bubble Tea Commands를 조합한 패턴이다:

1. `l` 키 → goroutine 생성 + 채널 반환 + `waitForLogCmd(ch)` 예약
2. `waitForLogCmd`: 채널에서 한 줄을 블록 대기, 도착하면 `logLineMsg`로 반환
3. `Update`가 `logLineMsg` 수신 → 로그에 추가 + 다시 `waitForLogCmd` 예약
4. `Esc` 키 → `context.CancelFunc()` 호출 → goroutine 종료 → 채널 close

```go
func waitForLogCmd(ch <-chan docker.LogLine) tea.Cmd {
    return func() tea.Msg {
        line, ok := <-ch
        if !ok {
            return nil // 채널 닫힘, 스트림 종료
        }
        return logLineMsg(line.Text)
    }
}
```

Docker의 multiplexed 로그 스트림은 각 줄 앞에 8바이트 헤더(스트림 타입 1B + 패딩 3B + 길이 4B)를 붙인다. `logs.go`에서 이를 벗겨낸다:

```go
if len(line) > 8 {
    line = line[8:]
}
```

### 4-6. Docker 이벤트 스트리밍 (v0.2.0)

Events 패널은 Docker 데몬의 컨테이너 라이프사이클 이벤트(start, stop, die, kill, restart 등)를 실시간으로 수신한다. 로그 스트리밍과 동일한 채널 기반 패턴을 사용한다:

```go
// events.go — Docker Events API를 채널로 감싼다
func (c *Client) StreamEvents(ctx context.Context) <-chan EventInfo {
    ch := make(chan EventInfo, 64)
    go func() {
        defer close(ch)
        f := filters.NewArgs()
        f.Add("type", "container")
        since := strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10)
        msgCh, errCh := c.cli.Events(ctx, events.ListOptions{
            Filters: f,
            Since:   since, // 앱 시작 전 1시간 과거 이벤트도 재생
        })
        // ...
    }()
    return ch
}
```

`Since` 파라미터로 앱 시작 전 1시간의 이벤트를 백필한다. 앱을 열었을 때 Events 탭이 비어있지 않고 직전 사건들이 이미 표시된다.

이벤트는 최신순으로 누적하고 100개로 제한한다:

```go
case eventMsg:
    ei := docker.EventInfo(msg)
    m.events = append([]docker.EventInfo{ei}, m.events...)
    if len(m.events) > 100 {
        m.events = m.events[:100]
    }
    return m, waitForEventCmd(m.eventCh)
```

이벤트 스트리밍은 Events 탭 최초 방문 시 시작된다. 이미 스트리밍 중이면 중복 시작하지 않는다:

```go
if m.activePanel == PanelEvents && m.eventCancel == nil {
    ctx, cancel := context.WithCancel(context.Background())
    m.eventCh = m.docker.StreamEvents(ctx)
    m.eventCancel = cancel
    return m, waitForEventCmd(m.eventCh)
}
```

### 4-7. 삭제 확인 오버레이와 cursor drift 버그

`d` 키 → `confirmDelete = true` → View가 전체 화면을 확인 다이얼로그로 교체.

초기 구현에서 버그가 있었다: 다이얼로그가 열려있는 동안 2초 tick이 발생하면 컨테이너 목록이 갱신되고 cursor가 가리키는 항목이 바뀔 수 있었다. `y`를 누르면 원래 선택한 컨테이너가 아닌 다른 컨테이너가 삭제될 수 있었다.

해결: 다이얼로그를 열 때 즉시 ID를 `pendingDeleteID`에 캡처한다:

```go
case keyMatches(msg, km.Delete):
    if m.activePanel == PanelContainers && len(m.containers) > 0 {
        m.pendingDeleteID = m.containers[m.cursor].ID // 즉시 캡처
        m.confirmDelete = true
    }

// y 눌렀을 때는 cursor가 아닌 캡처된 ID를 사용
case "y", "Y":
    id := m.pendingDeleteID
    m.pendingDeleteID = ""
    return m, removeContainerCmd(m.docker, id)
```

### 4-8. Image Pull 진행 상황

`dockviz pull nginx:alpine`은 별도의 Bubble Tea 프로그램(`internal/tui/pull.go`)으로 구현된다. Docker의 이미지 pull은 레이어 단위로 병렬 진행되고, 각 레이어의 상태를 JSON 이벤트 스트림으로 전달한다.

레이어 순서 유지가 문제였다. Go의 map은 순서가 없기 때문에 삽입 순서를 별도 슬라이스로 관리했다:

```go
layerOrder := []string{}
layers := map[string]*LayerStatus{}

if _, ok := layers[evt.ID]; !ok {
    layerOrder = append(layerOrder, evt.ID)
    layers[evt.ID] = &LayerStatus{ID: evt.ID}
}
```

### 4-9. 빌드 시간 버전 주입

```bash
go build -ldflags="-X main.version=v0.2.0" -o dockviz .
```

`main.go`의 `var version = "dev"`가 릴리스 빌드에서 실제 태그 버전으로 교체된다. GitHub Actions에서 태그 푸시 시 자동으로 처리되어 TUI 타이틀 바에 표시된다.

---

## 5. 패키지 설계 원칙

```
internal/docker/   — Docker API 접근만 담당. TUI에 대한 의존성 없음
internal/tui/      — UI 상태와 이벤트 처리만 담당. DockerClient 인터페이스만 알고 있음
internal/ui/       — 순수 렌더링 유틸리티. 스타일과 그래프 변환 함수만 포함
cmd/               — CLI 진입점. 두 레이어를 연결하는 얇은 접착제
```

`internal/` 아래에 두는 것은 외부 패키지 import를 Go 컴파일러가 차단하는 관례다.

---

## 6. 기술적으로 흥미로운 지점들

### Bubble Tea의 단방향 데이터 흐름

`Update`는 입력을 받아 새 상태를 반환할 뿐이다. 부작용(I/O, goroutine 시작)은 모두 `Cmd` 타입으로 명시적으로 표현된다. 실시간 데이터가 여러 경로로 들어오는 이 프로젝트에서 상태 불일치가 발생하지 않는 이유다.

### goroutine 누수 방지

로그와 이벤트 두 개의 스트리밍 goroutine이 동시에 존재할 수 있다. `context.CancelFunc`를 Model에 보관하고 앱 종료와 뷰 전환 시 항상 호출한다:

```go
case keyMatches(msg, km.Quit):
    if m.logCancel != nil {
        m.logCancel()
    }
    if m.eventCancel != nil {
        m.eventCancel()
    }
    return m, tea.Quit
```

### lazydocker와의 차별점

lazydocker는 컨테이너별 로그와 stats를 보여준다. dockviz가 추가로 보여주는 것은 Docker 데몬 수준의 이벤트 타임라인이다. 컨테이너가 왜 죽었는지, 언제 재시작됐는지, 어떤 순서로 사건이 발생했는지를 시간 순으로 볼 수 있다. SSH로만 접근 가능한 서버에서 브라우저 없이 이 뷰를 제공하는 터미널 도구는 없었다.

---

## 7. 의존성 요약

| 의존성 | 용도 | 선택 이유 |
|--------|------|-----------|
| `charmbracelet/bubbletea` | TUI 이벤트 루프 | TEA 패턴, 동시성 안전 |
| `charmbracelet/lipgloss` | 터미널 스타일링 | 선언적 API, 색상 추상화 |
| `charmbracelet/bubbles` | Spinner, KeyBinding | Bubble Tea 공식 컴포넌트 |
| `docker/docker` | Docker API | 공식 SDK, API 버전 자동 협상 |
| `spf13/cobra` | CLI 프레임워크 | 서브커맨드, 플래그 파싱 |

직접 의존성 5개. Go 모듈 시스템이 간접 의존성을 `go.sum`으로 고정한다.
