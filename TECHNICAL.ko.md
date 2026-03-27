# dockviz-cli — 기술 심층 분석

> 언어 선택, 라이브러리 결정, 구현 흐름, 그리고 그 과정에서 마주친 기술적 문제들

---

## 1. 왜 Go인가

### 컴파일 특성

Go는 **단일 바이너리**로 컴파일된다. `go build`를 실행하면 런타임, 표준 라이브러리, 의존성이 하나의 실행 파일로 묶인다. 이것이 도구성 CLI 프로젝트에서 가장 중요한 특성이다. `dockviz`를 서버에 배포하는 데 필요한 것은 바이너리 파일 하나뿐이다. Python이라면 `pip install`, Node라면 `node_modules` — 그런 것이 없다.

멀티플랫폼 크로스 컴파일도 Go의 강점이다. 환경변수 두 개로 끝난다:

```bash
GOOS=linux   GOARCH=amd64  go build -o dockviz-linux-amd64
GOOS=darwin  GOARCH=arm64  go build -o dockviz-darwin-arm64
GOOS=windows GOARCH=amd64  go build -o dockviz-windows-amd64.exe
```

GitHub Actions에서 matrix 전략으로 6개 플랫폼(linux/darwin/windows × amd64/arm64) 바이너리를 한 번에 뽑는다. C/C++에서 크로스 컴파일이 얼마나 고통스러운지 생각하면 이 단순함은 상당한 이점이다.

### goroutine과 채널

TUI는 본질적으로 동시성 문제다. 화면을 그리는 메인 루프, 2초마다 Docker 데이터를 fetch하는 타이머, 컨테이너 로그를 실시간으로 받아오는 스트림이 동시에 돌아야 한다. Go의 goroutine은 OS 스레드보다 훨씬 가볍고(스택 초기 2KB), 채널로 goroutine 간 통신을 명시적으로 제어할 수 있다. 이 프로젝트에서 이 특성을 직접적으로 활용했다.

### Docker SDK

Docker는 Go로 작성되었고, Docker SDK for Go(`github.com/docker/docker/client`)는 Docker가 직접 관리하는 공식 클라이언트다. HTTP API를 직접 호출하는 것보다 타입 안정성이 보장되고, API 버전 협상(`client.WithAPIVersionNegotiation()`)도 자동으로 처리된다.

---

## 2. 핵심 라이브러리 선택 이유

### Bubble Tea — The Elm Architecture for TUI

TUI 프레임워크 선택지는 여럿 있다. `tview`(Go), `termui`(Go), `rich`(Python), `textual`(Python) 등. 이 중 Bubble Tea를 선택한 이유는 **아키텍처 모델** 때문이다.

Bubble Tea는 Elm 언어의 아키텍처(TEA)를 Go로 구현한다:

```
Model  — 앱의 전체 상태 (구조체 하나)
Update — 메시지를 받아 새 Model을 반환하는 순수 함수
View   — Model을 받아 문자열을 반환하는 순수 함수
```

상태가 단일 `Model` 구조체에만 존재하고, 상태 변경은 오직 `Update` 함수를 통해서만 일어난다. 상태 변경 경로가 하나뿐이기 때문에 버그 추적이 쉽고, 렌더링은 항상 현재 상태의 함수이므로 UI 불일치가 발생하지 않는다.

비교 대상인 `tview`는 위젯 기반으로 상태가 여러 위젯 객체에 분산된다. Bubble Tea는 이를 중앙화한다. 실시간 데이터가 들어오고 키 입력이 발생하는 복잡한 대시보드에서 이 차이는 크다.

**Commands** 패턴도 중요하다. Bubble Tea에서 I/O는 `Cmd` 타입의 함수로 표현된다. `Update`는 I/O를 직접 수행하지 않고 "이 작업을 나중에 실행해라"는 `Cmd`를 반환한다. Bubble Tea 런타임이 goroutine에서 이를 실행하고 결과를 다시 `Update`로 전달한다. 덕분에 `Update`는 순수 함수로 유지된다.

```go
// tickMsg가 오면 데이터 fetch를 예약하고, 다음 tick도 예약한다.
case tickMsg:
    return m, tea.Batch(fetchDataCmd(m.docker), tickCmd())
```

### Lip Gloss — 스타일 선언

터미널 색상과 레이아웃을 `fmt.Sprintf("\033[32m%s\033[0m", text)` 방식으로 직접 ANSI 코드를 박아넣으면 코드가 지저분해지고 유지보수가 어렵다. Lip Gloss는 CSS와 유사한 선언적 스타일을 제공한다:

```go
TitleStyle = lipgloss.NewStyle().
    Bold(true).
    Foreground(ColorBlue).
    Padding(0, 1)
```

색상 팔레트를 `styles.go` 한 파일에 모아두어 테마 변경이 하나의 파일 수정으로 끝난다.

### Cobra — CLI 구조

`dockviz` 자체는 단순하지만 `dockviz pull <image>` 서브커맨드가 있다. Cobra는 서브커맨드, 플래그 파싱, 자동 help 생성을 제공하는 Go 표준 CLI 라이브러리다(kubectl, Hugo, GitHub CLI 등이 Cobra 기반이다).

`--demo` 플래그와 `pull` 서브커맨드를 자연스럽게 수용하면서 향후 서브커맨드 추가에도 대응 가능한 구조를 Cobra가 제공한다.

---

## 3. 전체 구현 흐름

### 3-1. 프로젝트 스캐폴딩과 인터페이스 설계

가장 먼저 한 일은 `DockerClient` 인터페이스를 정의하는 것이었다(`internal/docker/interface.go`). 실제 Docker 데몬에 연결하는 `Client`와 데모용 가짜 데이터를 생성하는 `DemoClient` 모두 이 인터페이스를 구현한다.

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
    Close()
}
```

TUI 코드(`internal/tui/`)는 인터페이스에만 의존한다. 덕분에 Docker가 없는 환경에서도 `--demo` 플래그 하나로 모든 기능을 시연할 수 있다.

### 3-2. 컨테이너 패널과 실시간 데이터

기본 컨테이너 목록을 구현한 후, 2초 주기 자동 갱신을 추가했다. Bubble Tea의 `tea.Tick`을 사용한다:

```go
func tickCmd() tea.Cmd {
    return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

`tickMsg`가 오면 `fetchDataCmd`를 실행하고 동시에 다음 `tickCmd`를 예약한다. 데이터 fetch는 goroutine에서 비동기로 실행되므로 메인 루프(UI 렌더링)를 블록하지 않는다.

CPU 사용량 계산은 Docker Stats API의 deltaV 공식을 그대로 구현했다:

```go
func calcCPUPercent(stats container.StatsResponse) float64 {
    cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
        float64(stats.PreCPUStats.CPUUsage.TotalUsage)
    sysDelta := float64(stats.CPUStats.SystemUsage) -
        float64(stats.PreCPUStats.SystemUsage)
    numCPU := float64(stats.CPUStats.OnlineCPUs)
    // ...
    return (cpuDelta / sysDelta) * numCPU * 100.0
}
```

Docker는 절대 나노초 값을 준다. 이전 snapshot과의 delta를 전체 시스템 delta로 나눠야 퍼센트가 나온다.

### 3-3. CPU Sparkline

컨테이너별로 최근 10개 CPU 값을 `map[string][]float64` 히스토리에 쌓는다. 새 데이터가 도착할 때마다(`dataMsg`) 히스토리를 업데이트하고, 10개를 초과하면 앞을 버린다:

```go
h = append(h, c.CPUPerc)
if len(h) > 10 {
    h = h[len(h)-10:]
}
```

`Sparkline()` 함수는 0~100 값들을 `▁▂▃▄▅▆▇█` 8단계 블록 문자로 변환한다. 최댓값을 기준으로 정규화하기 때문에 낮은 CPU에서도 상대적인 변화를 볼 수 있다.

### 3-4. ANSI 컬럼 정렬 문제

가장 까다로운 버그 중 하나였다. 터미널 테이블 컬럼을 `fmt.Sprintf("%-10s", value)`로 정렬할 때, ANSI 색상 코드가 포함된 문자열은 `len()`이 시각적 너비보다 훨씬 크다. 예를 들어 `\033[32m●running\033[0m`의 바이트 길이는 시각적 문자 수보다 훨씬 길어서 `%s` 포맷터가 잘못된 패딩을 계산한다.

해결 방법: **색상 입히기 전에 먼저 패딩을 적용**한다.

```go
// 틀린 방식: 색상 코드가 포함된 상태에서 width 지정
statusStr := fmt.Sprintf("%-12s", ui.StatusStyle(c.Status).Render("● running"))

// 올바른 방식: 평문 패딩 먼저, 색상 나중
statusText := fmt.Sprintf("%-12s", "● "+c.Status)  // 12자리 패딩
statusStr  := ui.StatusStyle(c.Status).Render(statusText)  // 그 다음 색상
```

Sparkline도 같은 원칙이다. `ui.Sparkline()`이 항상 10 rune 너비의 문자열을 반환하고, 그 다음에 `lipgloss.NewStyle().Foreground(...).Render(spark)`를 적용한다.

### 3-5. 실시간 로그 스트리밍

로그 스트리밍은 채널과 Bubble Tea Commands를 조합한다. 패턴은 다음과 같다:

1. `l` 키 입력 → goroutine 생성 + 채널 반환 + `waitForLogCmd(ch)` 예약
2. `waitForLogCmd`: 채널에서 한 줄을 블록 대기, 도착하면 `logLineMsg`로 반환
3. `Update`가 `logLineMsg` 수신 → 로그에 추가 + 다시 `waitForLogCmd` 예약 (재귀적 폴링)
4. `Esc` 키 입력 → `context.CancelFunc()` 호출 → goroutine 종료 → 채널 close → `waitForLogCmd`가 nil 반환

```go
func waitForLogCmd(ch <-chan docker.LogLine) tea.Cmd {
    return func() tea.Msg {
        line, ok := <-ch
        if !ok {
            return nil  // 채널 닫힘, 스트림 종료
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

### 3-6. Image Pull 진행 상황

`dockviz pull nginx:alpine` 명령은 별도의 Bubble Tea 프로그램(`internal/tui/pull.go`)으로 구현된다. Docker의 이미지 pull은 레이어 단위로 병렬 진행되고, 각 레이어의 진행 상황을 JSON 이벤트 스트림으로 전달한다.

`docker/pull.go`는 이 스트림을 파싱하여 레이어 ID별로 상태를 추적한다. 레이어 순서를 유지하기 위해 `map`과 함께 삽입 순서를 기록하는 `layerOrder []string`을 별도로 관리한다(Go의 map은 순서가 없다):

```go
layerOrder := []string{}
layers := map[string]*LayerStatus{}

// 새 레이어 발견 시
if _, ok := layers[evt.ID]; !ok {
    layerOrder = append(layerOrder, evt.ID)
    layers[evt.ID] = &LayerStatus{ID: evt.ID}
}
```

### 3-7. 삭제 확인 오버레이

`d` 키를 누르면 `confirmDelete bool` 플래그가 true가 된다. `View()`는 이 플래그가 true일 때 전체 화면을 확인 다이얼로그로 대체한다. `Update()`는 `confirmDelete`가 true일 때 모든 키 입력을 다이얼로그 전용 핸들러로 라우팅한다. 실수로 컨테이너가 삭제되는 상황을 방지하는 안전장치다.

### 3-8. Demo 모드

`DemoClient`(`internal/docker/demo.go`)는 `DockerClient` 인터페이스를 구현하며 미리 만들어진 가짜 데이터를 반환한다. 실제 Docker 데몬 없이 모든 기능(목록, 상태, 네트워크 토폴로지, 로그 스트리밍)을 시연할 수 있다.

이 설계 덕분에 `tui.Start(dc docker.DockerClient, version string)`는 어떤 구현이 들어오든 동일하게 동작한다:

```go
// cmd/root.go
if demo {
    dc = docker.NewDemoClient()
} else {
    dc, err = docker.NewClient()
}
tui.Start(dc, version)
```

### 3-9. 빌드 시간 버전 주입

바이너리가 자신의 버전을 알 수 있도록 `ldflags`로 컴파일 시점에 변수를 주입한다:

```bash
go build -ldflags="-X main.version=v0.2.3" -o dockviz .
```

`main.go`의 `var version = "dev"`가 릴리스 빌드에서는 실제 태그 버전으로 교체된다. GitHub Actions에서 태그 푸시 시 자동으로 이 과정이 실행된다.

### 3-10. CI/CD 파이프라인

GitHub Actions로 태그(`v*`) 푸시 시 자동 릴리스가 동작한다:

```
태그 푸시 (v0.2.3)
  → matrix build (6개 플랫폼)
  → GitHub Release 생성
  → 바이너리 업로드
```

각 플랫폼 바이너리는 `curl` 한 줄로 설치 가능하다.

---

## 4. 패키지 설계 원칙

```
internal/docker/   — Docker API 접근만 담당. TUI에 대한 의존성 없음
internal/tui/      — UI 상태와 이벤트 처리만 담당. Docker 인터페이스만 알고 있음
internal/ui/       — 순수 렌더링 유틸리티. 스타일과 그래프 변환 함수만 포함
cmd/               — CLI 진입점. 두 레이어를 연결하는 얇은 접착제
```

`internal/` 아래에 두는 것은 외부 패키지에서 import하지 못하도록 Go 컴파일러가 강제하는 관례다. 이 라이브러리들은 이 CLI 전용이다.

---

## 5. 기술적으로 흥미로운 지점들

### Bubble Tea의 단방향 데이터 흐름

이 프로젝트에서 가장 만족스러운 부분은 Bubble Tea의 순수 함수 모델이다. `Update`는 입력을 받아 새 상태를 반환할 뿐이다. 부작용(I/O, 고루틴 시작)은 모두 `Cmd` 타입으로 명시적으로 표현된다. 상태 버그를 추적할 때 "어디서 상태가 바뀌었나?"를 찾을 필요가 없다 — 항상 `Update`다.

### 채널 기반 로그 스트리밍의 goroutine 누수 방지

로그 뷰를 닫을 때 goroutine이 정리되지 않으면 계속 Docker로부터 데이터를 읽으면서 메모리를 먹는다. `context.CancelFunc`를 `Model`에 보관하고, 뷰를 닫거나 다른 컨테이너를 열기 전에 항상 호출한다:

```go
case keyMatches(msg, km.Logs):
    if m.logCancel != nil {
        m.logCancel()  // 기존 스트림 정리
    }
    ctx, cancel := context.WithCancel(context.Background())
    // ...
    m.logCancel = cancel
```

### 스파크라인 정규화

절대값이 아닌 상대값으로 표현한다. 컨테이너가 평소 0.1% CPU를 사용한다면, 0.5%로 올라갔을 때 스파크라인이 꽉 찬 막대로 표시된다. 절대값 기준이면 미세한 변화가 보이지 않는다. 모니터링에서는 상대적 변화가 더 의미 있는 경우가 많다.

---

## 6. 의존성 요약

| 의존성 | 용도 | 선택 이유 |
|--------|------|-----------|
| `charmbracelet/bubbletea` | TUI 이벤트 루프 | TEA 패턴, 동시성 안전 |
| `charmbracelet/lipgloss` | 터미널 스타일링 | 선언적 API, 색상 추상화 |
| `charmbracelet/bubbles` | Spinner, KeyBinding | Bubble Tea 공식 컴포넌트 |
| `docker/docker` | Docker API | 공식 SDK, API 버전 자동 협상 |
| `spf13/cobra` | CLI 프레임워크 | 서브커맨드, 플래그 파싱 |

직접 의존성 5개. Go 모듈 시스템이 간접 의존성을 `go.sum`으로 고정한다.
