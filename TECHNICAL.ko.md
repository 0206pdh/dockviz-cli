# dockviz-cli — 프로젝트 스펙 및 기술 심층 분석

> 제품 목표, 언어 선택, 라이브러리 결정, 아키텍처, 데이터 계약, 구현 흐름, 배포와 운영 제약을 소스 코드 기준으로 정리한 루트 기술 명세

이 문서는 단순 사용법을 나열하는 README가 아니다. `dockviz-cli`를 왜 Go로 만들었는지, 각 패키지가 어떤 책임을 갖는지, Docker 데이터가 어떻게 TUI 상태로 변환되는지, 기능을 추가할 때 어떤 규칙을 지켜야 하는지를 설명한다. 구현이 변경되면 이 문서도 함께 갱신한다.

현재 모듈 기준 언어 버전은 Go 1.25.0이며, 직접 의존성은 Bubble Tea, Bubbles, Lip Gloss, Docker SDK for Go, Cobra다.

---

## 0. 프로젝트 한눈에 보기

`dockviz-cli`는 Docker 환경을 터미널에서 실시간으로 관찰하고 기본 조작까지 수행하는 TUI(Text User Interface) 애플리케이션이다.

```text
입력                 출력/효과
────────────────────────────────────────────────────────
Docker daemon        컨테이너·네트워크·이미지 목록
Docker stats         CPU·메모리 현재값과 히스토리
Docker events        라이프사이클 이벤트·장애 정보
Docker logs          마지막 50줄 + 실시간 로그
Docker ImagePull     레이어별 다운로드 진행률
키보드 입력          start/stop/delete/detail/logs/chart/exec
```

### 제품 목표

- `docker ps`, `docker stats`, `docker logs`, `docker events`, `docker images`, `docker network inspect`를 여러 터미널에서 번갈아 실행해야 하는 불편을 줄인다.
- SSH로만 접근하는 서버에서도 브라우저나 별도 웹 서버 없이 동작한다.
- CPU·메모리의 현재값뿐 아니라 최근 추세를 보여준다.
- 네트워크 토폴로지와 이벤트 타임라인을 함께 보여 장애의 시작 지점을 찾도록 돕는다.
- Docker가 없는 환경에서도 `--demo`로 화면과 상호작용을 체험할 수 있게 한다.
- 운영 서버에 런타임을 추가 설치하지 않고 단일 바이너리로 배포한다.

### 현재 범위

컨테이너 목록/통계/상세/로그/셸/start/stop/remove, 네트워크와 토폴로지, 이미지 태그 목록/삭제, 이벤트 스트림, 이미지 Pull 진행률, 로컬·원격 Docker endpoint, 데모 모드, Linux·macOS·Windows 배포를 지원한다.

Compose 파일 편집, Kubernetes/Swarm 클러스터 관리, 장기 메트릭 저장소, 여러 Docker daemon의 동시 통합 모니터링, 웹 UI는 현재 범위가 아니다.

---

## 1. 왜 Go인가

언어 선택은 취향보다 이 프로그램의 실행 환경과 기능 조합에서 결정했다. 이 도구의 핵심 조건은 **터미널 애플리케이션**, **Docker API 클라이언트**, **여러 실시간 스트림**, **운영 서버에 쉽게 배포되는 실행 파일**이다.

### 단일 바이너리 배포

Go 프로그램은 애플리케이션과 필요한 Go 라이브러리를 실행 파일로 묶어 배포할 수 있다. 이 프로젝트에서 운영 서버에 필요한 것은 일반적으로 `dockviz` 바이너리 하나다. Python의 interpreter/virtualenv나 Node의 runtime/`node_modules`를 별도로 준비하지 않아도 된다. SSH로만 접근하는 서버에서 `curl` 한 줄로 설치되는 툴을 만드는 데 이 특성이 핵심이다.

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

### 언어 선택의 한계까지 포함한 판단

Go가 동시성과 배포를 자동으로 해결하는 것은 아니다. 이 프로젝트가 직접 책임져야 하는 문제는 stream 종료·재연결, context 취소, Docker stats delta 계산, ANSI 문자열의 표시 너비, 네트워크 inspect race, 원격 `docker exec` CLI 환경이다. 따라서 언어 선택의 이점은 `DockerClient` 인터페이스, TEA 상태 흐름, 명시적 context 수명 관리와 결합될 때 실제 제품 품질로 이어진다.

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

`Sparkline()` 함수는 값들을 `▁▂▃▄▅▆▇█` 8단계 블록 문자로 변환한다. 값은 컨테이너 간 최댓값이 아니라 **고정된 0~100% 기준**으로 매핑된다. 따라서 다른 컨테이너의 부하에 따라 같은 컨테이너의 막대가 왜곡되지 않는다. 입력이 100을 넘으면 100으로 clamp한다.

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

Docker의 non-TTY multiplexed 로그 스트림은 각 프레임 앞에 8바이트 헤더(스트림 타입 1B + 패딩 3B + 길이 4B)를 붙인다. `logs.go`는 바이트를 임의로 잘라내지 않고 Docker SDK의 `stdcopy.StdCopy`로 stdout/stderr를 역다중화한 뒤 `bufio.Scanner`로 줄을 나눈다.

```go
stdcopy.StdCopy(pw, pw, rc)
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

이벤트 스트리밍은 `newModel()`에서 즉시 시작된다. 따라서 Events 탭을 최초 방문하기 전의 과거 이벤트도 timeline과 topology 상태에 반영된다. stream이 끊긴 뒤에만 `r` 키로 기존 context를 취소하고 새 stream을 만든다:

```go
if m.eventDisconnected {
    m.eventCancel()
    ctx, cancel := context.WithCancel(context.Background())
    m.eventCh = m.docker.StreamEvents(ctx)
    m.eventCancel = cancel
    m.eventDisconnected = false
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

---

## 8. 제품 동작 명세

### 8-1. CLI 계약

```bash
dockviz
dockviz --demo
dockviz --host tcp://192.168.1.100:2375
dockviz pull nginx:alpine
dockviz --version
```

| 명령/플래그 | 동작 |
|---|---|
| `dockviz` | 로컬 socket 또는 `DOCKER_HOST`의 Docker daemon에 연결해 대시보드 실행 |
| `--demo` | Docker daemon 없이 가상 컨테이너·네트워크·이벤트·로그 실행 |
| `--host <endpoint>` | Docker endpoint를 지정하며 비어 있지 않으면 `DOCKER_HOST`보다 우선 |
| `pull <image>` | 실제 Docker ImagePull stream을 레이어별 Pull TUI로 표시 |
| `--version` | 빌드 시 `ldflags`로 주입된 버전 출력 |

`pull`은 `cobra.ExactArgs(1)`을 사용하므로 image reference를 정확히 하나만 받는다. Pull 화면은 기본 대시보드와 독립적인 Bubble Tea program이다. `--demo`는 대시보드의 가상 데이터 모드이며 실제 이미지 다운로드를 흉내 내지 않는다.

### 8-2. 기본 화면과 보조 화면

```text
Dashboard
  ├─ Containers  목록, 통계, sparkline
  ├─ Networks    topology + 선택 network 이벤트
  ├─ Images      tag별 이미지 목록
  └─ Events      컨테이너 라이프사이클 timeline

보조 화면
  ├─ Detail      컨테이너 상세·volume
  ├─ Logs       마지막 50줄 + follow stream
  └─ Chart       CPU/MEM 최근 60개 샘플
```

패널은 `Tab`으로 `Containers → Networks → Images → Events` 순환한다. 대시보드는 `tea.WithAltScreen()`으로 실행되어 셸의 기존 scrollback을 오염시키지 않는다.

### 8-3. 키보드 계약

| 키 | 동작 | 조건 |
|---|---|---|
| `q`, `Ctrl+C` | 종료, 로그·이벤트 context 취소 | 전체 |
| `Tab` | 패널 전환 | Dashboard |
| `↑`, `k` | 위로 이동 | 목록·로그 |
| `↓`, `j` | 아래로 이동 | 목록·로그 |
| `Enter` | 컨테이너 상세 | Containers |
| `Esc` | 뒤로 가기·로그 종료·삭제 취소 | 상황별 |
| `s` | 실행 중이면 stop, 그 외 start | Containers |
| `d` | 삭제 확인 오버레이 | Containers·Images |
| `l` | 로그 stream 시작 | Containers |
| `r` | fetch 실행, 끊긴 event stream 재연결 | Dashboard |
| `g` | CPU/MEM history 차트 | Containers |
| `e` | 실행 중 컨테이너의 셸 | 실제 Containers |

### 8-4. 좁은 터미널 대응

Networks는 80열 이상이면 좌우 분할을 사용한다.

```text
왼쪽: Topology                   오른쪽: Events — 선택한 network
컨테이너 노드와 연결선             해당 network 컨테이너의 event만 필터링
```

80열 미만이면 단순 네트워크 표(`NETWORK`, `DRIVER`, `SUBNET`, `CTRS`)로 fallback한다. 이 분기는 첫 `WindowSizeMsg` 전처럼 width가 아직 0인 경우에도 적용된다.

---

## 9. 데이터 모델과 Docker API 매핑

### 9-1. ContainerInfo

```go
type ContainerInfo struct {
    ID      string
    Name    string
    Image   string
    Status  string
    CPUPerc float64
    MemMB   float64
    Ports   string
    Volumes []string
}
```

`ListContainers`는 `All: true`로 호출해 running과 stopped container를 모두 가져온다.

- 이름의 선행 `/`를 제거한다.
- ID는 최대 12자리로 줄인다.
- public port가 있으면 `public:private`, 없으면 private port만 표시한다.
- 같은 포트 문자열은 deduplicate한다.
- mount는 `source → destination`으로 변환한다.
- named volume은 `Mount.Name`, bind mount는 `Mount.Source`를 source로 사용한다.
- read-only mount는 `(ro)`를 추가한다.

실행 중 container의 stats는 목록 조회와 별도의 `ContainerStats(..., false)` snapshot으로 가져온다. 화면은 running이 아닐 때 CPU와 MEM을 `-`로 표시한다.

### 9-2. CPU 계산 계약

Docker stats가 제공하는 누적값으로 다음을 계산한다.

```text
cpuDelta    = current CPUUsage.TotalUsage - previous TotalUsage
systemDelta = current SystemUsage - previous SystemUsage
numCPU      = OnlineCPUs, 단 0이면 len(PercpuUsage)
CPU%        = (cpuDelta / systemDelta) × numCPU × 100
```

`systemDelta == 0`이면 0을 반환한다. CPU%는 여러 코어 사용량을 포함하므로 100을 넘을 수 있다. 목록 sparkline은 0~100 고정 scale이고, full chart는 실제 최대값을 기준으로 y축을 확장한다.

### 9-3. NetworkInfo

```go
type NetworkInfo struct {
    ID         string
    Name       string
    Driver     string
    Subnet     string
    Containers []ContainerEndpoint
}

type ContainerEndpoint struct {
    Name string
    IPv4 string
}
```

`NetworkList`는 연결 container를 채우지 않으므로 network마다 `NetworkInspect`를 호출한다.

- IPAM 설정의 첫 번째 subnet을 사용한다.
- `172.20.0.2/16`에서 `/16`을 제거해 IPv4를 표시한다.
- inspect 중 network가 삭제된 `NotFound` race는 해당 항목을 건너뛴다.
- 그 외 inspect 오류는 fetch 전체 오류로 반환한다.
- inspect map은 이름순으로 정렬한다.
- `bridge`, `host`, `none`은 시스템 네트워크 우선순위를 적용하고 나머지는 이름순이다.

### 9-4. ImageInfo

```go
type ImageInfo struct {
    ID      string
    Tag     string
    AllTags []string
    SizeMB  float64
}
```

이미지 한 개에 여러 repository tag가 있으면 tag마다 한 행을 생성한다. 같은 image ID의 `AllTags`를 각 행에 보관하는 이유는 삭제 확인 시 “현재 tag만 제거되고 이미지 자체는 다른 tag가 남아 유지될 수 있음”을 설명하기 위해서다.

- `sha256:` 접두사는 제거한다.
- tag가 없는 이미지는 `<none>` 행으로 만든다.
- `ImageList(All: false)` 결과를 사용한다.
- tag 알파벳순으로 정렬한다.
- 삭제는 `ImageRemove(Force: false)`다.
- 1024MB 이상은 GB, 미만은 MB로 포맷한다.

### 9-5. 이벤트와 상태

```go
type EventInfo struct {
    Time          time.Time
    Action        string
    ContainerName string
    ContainerID   string
    ExitCode      int
    OOMKilled     bool
    Disconnected  bool
}

type ContainerState struct {
    Status       string
    ExitCode     int
    OOMKilled    bool
    RestartCount int
    UpdatedAt    time.Time
}
```

`StreamEvents`는 `type=container` filter와 시작 시점 이전 1시간의 `Since`를 사용한다. TUI는 수신 이벤트를 newest-first로 앞에 삽입하고 최대 100개를 보관한다.

| action | `ContainerStates` 전이 |
|---|---|
| `start` | `running`, restart count를 새 상태로 초기화 |
| `restart` | `restarting`, 기존 count 증가 |
| `die` | `dead`, exit code와 `OOMKilled` 기록 |
| `destroy` | container name key 삭제 |
| 기타 | timeline에는 저장하지만 topology 상태는 변경하지 않음 |

상태 map의 key는 container ID가 아니라 name이다. 이벤트가 아직 없는 노드는 회색 `○`, running은 초록 `●`, restarting은 노랑 `◑`, dead는 빨강 `✗`로 렌더링한다.

---

## 10. TUI Model과 데이터 흐름

### 10-1. Model이 관리하는 상태

```text
연결/빌드      docker, host, demo, version, loading, err
현재 snapshot  containers, networks, images
탐색           activePanel, activeView, cursor, selectedID
레이아웃       width, height
삭제           confirmDelete, pendingDeleteID
히스토리       history, memHistory
로그           logs, logScroll, logCh, logCancel
이벤트         events, eventCh, eventCancel, eventDisconnected
토폴로지       ContainerStates
```

Model이 애플리케이션의 single source of truth가 되므로 View는 API를 호출하거나 별도 mutable 상태를 만들지 않는다.

### 10-2. 초기화

`newModel`은 다음을 수행한다.

1. spinner를 만든다.
2. CPU/MEM history map을 만든다.
3. event context와 `StreamEvents` channel을 만든다.
4. panel을 Containers, view를 Dashboard, cursor를 0으로 초기화한다.

따라서 이벤트 stream은 Events 탭을 처음 방문할 때가 아니라 애플리케이션 시작부터 시작된다. 과거 1시간 event가 먼저 들어오면 topology의 마지막 상태도 초기 화면에서 구성된다.

`Init`은 spinner tick, 첫 data fetch, 2초 tick, 첫 event 대기를 `tea.Batch`로 예약한다.

### 10-3. 2초 갱신 흐름

```text
tickMsg
  → fetchDataCmd + 다음 tickCmd 예약
  → ListContainers / ListNetworks / ListImages 병렬 실행
  → 모든 목록 완료
  → running container별 FetchStats 병렬 실행
  → dataMsg
  → Model snapshot 교체
  → cursor clamp
  → CPU/MEM history에 샘플 추가
  → View 재렌더링
```

목록 세 가지 중 하나가 실패하면 `dataMsg.err`로 전체 fetch를 실패시킨다. 반대로 개별 container stats 실패는 목록을 가리지 않으며 해당 stats만 기본값으로 남긴다. history는 container ID별로 최대 60개이고, 2초 주기 기준 약 2분이다. Containers 표의 sparkline은 그중 최신 10개만 사용한다.

### 10-4. 메시지와 Command

```text
tea.KeyMsg        사용자 키
tea.WindowSizeMsg terminal 크기
spinner.TickMsg   spinner animation
tickMsg           polling timer
dataMsg           snapshot fetch 결과
eventMsg          EventInfo 하나
logLineMsg        로그 한 줄
execDoneMsg       docker exec 종료
pullEventMsg      Pull TUI의 레이어 snapshot
```

I/O는 `Update` 내부에서 직접 실행하지 않고 command로 감싼다. 예를 들어 `s` 입력은 `toggleContainerCmd`를 반환하고, command가 Docker API를 호출한 뒤 새 `dataMsg`를 반환한다.

---

## 11. 실시간 스트림 구현 명세

모든 stream은 다음 경계를 가진다.

```text
Docker SDK blocking response
  → docker package goroutine
  → 프로젝트 타입 channel
  → Bubble Tea wait Cmd
  → tea.Msg
  → Update에서 Model 변경
  → View 재렌더링
```

### 11-1. Events stream

`StreamEvents(ctx)`는 buffered channel을 만들고 Docker `Events`를 goroutine에서 읽는다.

- container type만 필터링한다.
- 과거 1시간 event를 함께 요청한다.
- actor name과 ID를 도메인 타입으로 변환한다.
- ID는 12자리로 줄인다.
- `die`의 `exitCode`, `oomKilled` attribute를 파싱한다.
- context가 취소되면 정상 종료한다.
- daemon/network 오류로 stream이 끊기면 `Disconnected: true` sentinel을 보낸다.

TUI는 disconnected sentinel을 timeline 데이터로 저장하지 않고 연결 끊김 표시만 바꾼다. `r`을 누르면 기존 event context를 취소하고 새 channel과 wait command를 만든다. 자동 재연결은 현재 구현의 동작이 아니며 명시적인 `r`이 재연결 트리거다.

### 11-2. Logs stream

`StreamLogs`는 `Tail: "50"`, `Follow: true`, stdout/stderr 활성화로 마지막 50줄과 이후 output을 가져온다.

```text
ContainerLogs response
  → io.Pipe
  → stdcopy.StdCopy로 multiplex 해제
  → bufio.Scanner로 줄 분리
  → LogLine{Text: line}
  → channel
```

`l`을 누르면 새 context를 만들고 `waitForLogCmd`가 첫 줄을 기다린다. 한 줄이 도착하면 `logLineMsg`를 Model에 추가하고 같은 channel을 다시 기다린다. `Esc`나 종료 시 `CancelFunc`를 호출하고 channel 소비를 중단한다. 현재 구현은 로그 화면을 닫기 전까지 받은 줄을 Model에 누적하므로 장시간 대량 로그는 메모리 사용량에 영향을 준다.

### 11-3. Image Pull stream

Docker Pull은 JSON line stream이다.

```json
{"status":"Downloading","id":"abc","progressDetail":{"current":123,"total":456}}
{"status":"Status: Downloaded newer image for nginx:alpine"}
```

Go map은 순서를 보장하지 않으므로 `layerOrder []string`와 `layers map[string]*LayerStatus`를 함께 사용한다. 처음 본 layer ID는 order에 추가하고 최신 상태는 map에 저장한다. 화면에 보낼 때 `snapshot`이 order 순서로 복사한다.

ID가 없는 line은 top-level status로 처리하며 `Status:`로 시작하면 완료로 판단한다. Pull TUI는 layer마다 20칸 bar, 퍼센트, current/total bytes, 상태를 렌더링하고 전체 다운로드 합계를 하단에 표시한다.

---

## 12. 사용자 조작의 안전성

### 12-1. 삭제 대상은 cursor가 아니라 ID를 캡처한다

자동 갱신 중 삭제 확인창이 떠 있는 동안 목록이 바뀔 수 있다. 확인 시점에 cursor를 다시 읽으면 원래 선택한 항목과 다른 항목을 삭제할 수 있다. 구현은 `d`를 누르는 순간 `pendingDeleteID`에 컨테이너 ID 또는 image tag를 저장한다.

```text
d
  → pendingDeleteID 캡처
  → confirmDelete = true
  → y/Y: 캡처한 ID/tag로 삭제
  → n/N/Esc: 취소
```

컨테이너 삭제는 `Force: true`이며 이미지 삭제는 `Force: false`다. 다중 tag 이미지에는 현재 tag만 제거되고 이미지 자체는 남을 수 있다는 경고를 표시한다.

### 12-2. start/stop

```text
status == running → ContainerStop(id), timeout 10초
그 외             → ContainerStart(id)
```

작업이 끝나면 즉시 fetch를 실행한다. 오류는 `dataMsg.err`로 전달된다.

### 12-3. 컨테이너 셸

셸은 Docker SDK exec API가 아니라 `tea.ExecProcess`로 로컬 Docker CLI를 실행한다.

```text
docker exec -it <name> sh -c "bash 2>/dev/null || sh"
```

`--host`가 있으면 `docker -H <host> exec ...`로 전달한다. Bubble Tea가 TUI를 일시 정지하고 서브프로세스에 터미널을 넘긴다. 프로세스가 종료되면 `execDoneMsg`가 들어오고 대시보드가 재개되면서 데이터를 다시 fetch한다. 실행 중인 실제 container에서만 가능하며 DemoClient에서는 no-op이다.

---

## 13. 오류, 취소, 리소스 수명

```text
NewClient/Ping 실패
  → tui.Start error
  → Cobra stderr 출력
  → 실패 exit code

목록 fetch 실패
  → dataMsg.err
  → Model.err
  → Error 화면

개별 stats 실패
  → 목록은 유지
  → 해당 stats만 기본값

event stream 오류
  → Disconnected sentinel
  → [r]로 재연결

log/Pull 오류
  → channel 종료 또는 typed error
```

Model은 event와 log 각각의 `CancelFunc`를 보관한다.

- 종료 시 두 context를 취소한다.
- 새 로그를 열기 전 이전 log context를 취소한다.
- Logs에서 Esc를 누르면 log context를 취소하고 Dashboard로 돌아간다.
- event 재연결 시 기존 context를 취소한 후 새 context를 만든다.

목록 갱신 후 cursor는 `activeListLen()` 범위로 clamp한다. 빈 목록이나 목록 축소에서도 index가 유효하지 않도록 하기 위한 방어 코드다.

---

## 14. 패키지별 책임과 변경 규칙

```text
main.go                 최소 진입점, version 전달
cmd/                    Cobra 명령·플래그·서브명령
internal/docker/        Docker SDK 접근과 도메인 타입 변환
internal/tui/           Model·Update·View·비동기 command
internal/ui/            순수 시각화 유틸리티와 스타일
```

### 파일별 책임

| 파일 | 책임 |
|---|---|
| `internal/docker/client.go` | SDK client 생성, host/env, API 협상, Ping, Close |
| `containers.go` | 목록, stats, CPU 공식, start/stop/restart/remove |
| `networks.go` | list+inspect, subnet/endpoint, stable sorting |
| `images.go` | tag row 변환, sorting, 안전한 image remove |
| `events.go` | event filter, 1시간 백필, exit/OOM parsing, disconnect |
| `logs.go` | 마지막 50줄, follow, stdcopy 역다중화, line channel |
| `pull.go` | Pull JSON parser, layer order, progress event |
| `demo.go` | 실제 구현과 같은 계약의 가상 client |
| `model.go` | Model, panel/view, 초기화, 병렬 fetch |
| `update.go` | 메시지 처리, 키 전이, action command, stream wait |
| `view.go` | 모든 화면과 좁은 터미널 fallback |
| `tui/pull.go` | `pull` 전용 독립 TUI |
| `ui/styles.go` | 색상, 공통 style, 상태 icon, 고정 폭 sparkline |
| `ui/graph.go` | 네트워크 ASCII graph와 상태별 노드 |

새 Docker 기능은 `internal/docker`의 도메인 타입과 `DockerClient` 계약부터 추가한다. 실제 Client와 DemoClient가 모두 계약을 구현한 뒤 TUI command와 Model 메시지를 연결한다. View에서는 API를 호출하지 않는다. blocking I/O는 항상 command 또는 stream goroutine으로 격리한다.

---

## 15. 테스트와 검증

```bash
go test ./...
go vet ./...
go run . --demo
go build -o dockviz .
```

현재 테스트는 Docker daemon 없이 검증 가능한 순수 로직을 우선한다.

| 테스트 | 검증 대상 |
|---|---|
| `internal/docker/containers_test.go` | CPU delta, OnlineCPUs fallback, zero system delta, port formatting/dedup |
| `internal/ui/styles_test.go` | sparkline 10 rune 폭, 0·100·범위 밖 값, 고정 scale |
| `internal/tui/update_test.go` | cursor clamp의 범위와 빈 목록 처리 |

추가 기능을 구현할 때 권장되는 테스트는 다음과 같다.

- container 이름·mount·port 변환
- network CIDR 제거, NotFound race, 정렬
- 여러 image tag 펼치기와 `AllTags` 경고
- action별 `ContainerState` 전이
- disconnected 이후 `r` 재연결
- log context 취소와 channel 종료
- Pull layer order 보존과 `Status:` 완료 처리
- 삭제 확인 중 refresh가 발생해도 `pendingDeleteID`가 유지되는지
- 80열 미만 Networks fallback
- DemoClient 기반 Model update smoke test

실제 Docker daemon을 사용하는 통합 테스트는 daemon 상태·권한·컨테이너와 이미지 자원을 요구하므로 순수 단위 테스트와 분리한다.

---

## 16. 배포와 패키징

### 16-1. 릴리스 workflow

`v*` tag push가 `.github/workflows/release.yml`을 트리거한다.

```text
build
  → Linux/macOS/Windows × amd64/arm64 바이너리
  → -s -w와 tag version ldflags

package_python
  → 각 바이너리를 플랫폼별 wheel에 포함

package_deb
  → Linux amd64/arm64 .deb

publish_pypi (PYPI_API_TOKEN이 있을 때)
  → wheel PyPI 업로드

deploy_apt_repo (APT_GPG_PRIVATE_KEY가 있을 때)
  → Packages/Release/GPG 서명 후 GitHub Pages

release
  → 바이너리·wheel·deb를 GitHub Release asset으로 업로드
```

### 16-2. Python wheel의 역할

Python 패키지는 Python으로 다시 작성한 앱이 아니다. `setup.py`가 릴리스에서 미리 빌드된 Go 바이너리를 `dockviz_cli` 패키지 안에 복사하고, `console_scripts`가 launcher를 노출한다.

```text
pip install dockviz
  → dockviz_cli._launcher:main
  → 현재 OS의 포함 바이너리 탐색
  → Unix: os.execv
  → Windows: subprocess.run
```

`DOCKVIZ_WHEEL_PLAT_NAME`으로 wheel platform tag를 지정하고 `root_is_pure = False`로 플랫폼 바이너리 패키지임을 표시한다. 이 구조는 Python 설치 채널을 제공하면서 실제 애플리케이션 구현은 Go 하나로 유지한다.

### 16-3. Debian과 APT

`packaging/build-deb.sh`는 바이너리를 `/usr/bin/dockviz`에 넣고 license를 `/usr/share/doc/dockviz/LICENSE`에 설치한다. 지원 architecture는 amd64와 arm64다.

`build-apt-repo.sh`는 `.deb`를 pool에 복사하고 architecture별 `Packages`, `Packages.gz`, `Release`를 만든다. `APT_GPG_PRIVATE_KEY`가 있으면 `InRelease`, `Release.gpg`, 공개 keyring을 함께 만든다.

---

## 17. 운영 제약과 설계상 주의점

### Docker 권한과 endpoint

로컬 Unix socket 접근 권한이 없거나 Docker daemon이 꺼져 있으면 `Ping`에서 시작이 실패한다. 원격 endpoint를 쓸 때는 `--host` 또는 `DOCKER_HOST`가 정확해야 하며, TCP daemon의 TLS·인증 설정은 Docker client 환경에 맞게 준비되어야 한다.

### 원격 셸의 별도 조건

목록·stats·logs·events·Pull은 Docker SDK endpoint를 사용하지만 셸 접속은 로컬 `docker` CLI를 실행한다. 따라서 원격 셸까지 사용하려면 로컬 Docker CLI가 설치되어 있고 동일 endpoint에 접근할 수 있어야 한다.

### 데이터 신선도

목록/stats는 2초 polling이고 logs/events는 stream이다. 그러므로 snapshot의 `Status`와 event-derived topology 상태 사이에 짧은 시간 차이가 생길 수 있다. 두 데이터는 서로 다른 목적의 사실이며, event가 도착하면 topology 상태를 갱신한다.

### 메모리와 표시 폭

이벤트는 최대 100개, container별 CPU/MEM history는 각각 최대 60개다. 로그는 현재 화면에 수신된 모든 줄을 누적한다. 유니코드 아이콘과 ANSI 색상이 포함되므로 padding은 반드시 색상을 적용하기 전에 수행한다. 좁은 터미널에서는 Networks fallback을 유지한다.

---

## 18. 설계 결정 요약

| 결정 | 이유 | 결과 |
|---|---|---|
| Go | 단일 바이너리, SDK, goroutine/channel, cross compile | 운영 서버 배포가 단순함 |
| Bubble Tea TEA | 입력·비동기 응답·렌더링을 단방향으로 통합 | 상태 변경 경로가 명확함 |
| `DockerClient` interface | 실제/데모/테스트 구현 분리 | TUI가 SDK에 직접 결합되지 않음 |
| polling + streaming | snapshot과 연속 데이터의 성격이 다름 | 데이터 종류별 적합한 갱신 |
| 병렬 fetch | 목록과 다수 stats의 지연을 줄임 | refresh 총 대기 시간 단축 |
| history 제한 | 장기 실행 시 무한 증가 방지 | 약 2분 chart 제공 |
| tag별 image row | 삭제 대상과 image 보존 관계를 설명하기 쉬움 | 다중 tag 안전 삭제 |
| ID 캡처 삭제 | refresh 중 cursor drift 방지 | 오삭제 위험 감소 |
| `stdcopy` | Docker multiplex framing 준수 | stdout/stderr 로그 보존 |
| DemoClient | Docker 없는 개발·CI·문서 화면 | 실제와 같은 UI 경로 검증 |
| Go binary + Python wheel | 설치 채널을 넓힘 | 구현은 Go 하나로 유지 |

---

## 19. 기능 추가 체크리스트

1. Docker API 접근이 필요하면 `internal/docker`에 프로젝트 도메인 타입을 먼저 정의한다.
2. 실제 Client와 DemoClient가 모두 `DockerClient` 계약을 만족하게 한다.
3. blocking I/O를 `Update`에서 직접 호출하지 않고 `tea.Cmd`로 감싼다.
4. channel을 만드는 goroutine의 context 취소와 종료 조건을 설계한다.
5. 새 응답은 typed `tea.Msg`로 바꾸고 Update에서만 Model을 변경한다.
6. View는 Model을 읽기만 하며 Docker API를 호출하지 않는다.
7. 색상 문자열은 평문 padding 후 렌더링한다.
8. 삭제·재시작 등 파괴적 동작은 확인창과 stable ID 캡처를 사용한다.
9. 외부 daemon 없이 검증 가능한 순수 함수와 단위 테스트를 추가한다.
10. CLI, 키 바인딩, README, 이 스펙 문서를 함께 갱신한다.
