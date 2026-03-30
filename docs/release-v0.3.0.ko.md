# v0.3.0 릴리즈 — 장애 전파 시각화 (Failure Propagation Visualizer)

## 개요

v0.3.0은 실시간 장애 전파 시각화 기능을 추가합니다. Networks 탭이 분할 패널 레이아웃으로 완전히 재설계되었습니다. 왼쪽의 컬러 토폴로지 그래프에서 컨테이너 상태를 한눈에 확인하고, 오른쪽의 네트워크별 이벤트 타임라인에서 정확히 언제 무슨 일이 있었는지 파악할 수 있습니다.

---

## 구현 과정

### 1단계 — 엔지니어링 리뷰 (`/plan-eng-review`)

코드 작성 전 전체 코드베이스 감사를 진행했습니다. 주요 발견 사항과 처리 결과:

| 발견 사항 | 수정 내용 |
|---------|----------|
| CPU/MEM 통계 항상 0 | `fetchDataCmd`에서 `FetchStats`를 두 번째 병렬 고루틴 패스로 실행 |
| 컨테이너/이미지 삭제 오류 묵음 처리 | `removeContainerCmd` / `removeImageCmd`가 `dataMsg{err}`로 오류 반환 |
| 이벤트 스트림이 탭 방문 시에만 시작 | `StreamEvents`를 `newModel()` + `Init()` 배치로 이동 |
| TTY 컨테이너 로그 깨짐 | 수동 8바이트 헤더 제거를 `stdcopy.StdCopy` + `io.Pipe`로 교체 |
| 스파크라인 스케일 오해 소지 | 고정 0–100% 스케일로 수정 (이전: 로컬 최댓값 기준 상대 스케일) |
| `RunE` 내부 `os.Exit` 호출 | `return fmt.Errorf(...)`으로 교체 |

모든 수정은 **PR #3**에서 반영됐습니다 (v0.3.0 이전 머지).

### 2단계 — v0.3.0 기능 구현

의존성 순서대로 6개 파일 변경:

#### `internal/docker/state.go` (신규 파일)

이벤트 스트림에서 파생된 컨테이너 최종 상태를 표현하는 `ContainerState` 구조체를 정의합니다. Docker API 폴링 방식이 아닌 이벤트 기반입니다.

```go
type ContainerState struct {
    Status       string    // "running" | "dead" | "restarting"
    ExitCode     int       // 0=정상 종료, 137=SIGKILL, 1=앱 오류
    OOMKilled    bool
    RestartCount int       // 세션 범위, 앱 재시작 시 초기화
    UpdatedAt    time.Time
}
```

**이 패키지에 둔 이유:** `internal/ui`와 `internal/tui` 모두 이미 `internal/docker`를 임포트합니다. 여기에 두면 `ui/graph.go`가 `tui` 타입을 참조할 때 생기는 임포트 사이클을 방지할 수 있습니다.

#### `internal/docker/events.go`

`EventInfo`에 두 필드 추가:

```go
ExitCode  int   // container/die 이벤트 시 채워짐
OOMKilled bool  // die 이벤트에서 oomKilled="true"일 때 채워짐
```

스트리밍 고루틴 내에서 `msg.Actor.Attributes`를 파싱해 채웁니다.

#### `internal/tui/model.go`

`Model`에 추가:

```go
ContainerStates map[string]docker.ContainerState
```

`newModel()`에서 빈 맵으로 초기화해 토폴로지 렌더러가 nil 맵을 받지 않도록 합니다.

#### `internal/tui/update.go`

`eventMsg` 핸들러에서 이벤트 누적 후 상태 전이 처리:

| 이벤트 액션 | 상태 전이 |
|------------|---------|
| `start` | `Status="running"`, RestartCount 0으로 초기화 |
| `restart` | `Status="restarting"`, RestartCount 증가 |
| `die` | `Status="dead"`, ExitCode/OOMKilled 채워짐 |
| `destroy` | 맵에서 항목 삭제 |

#### `internal/ui/graph.go`

`RenderNetworkGraph` 시그니처 변경:

```go
func RenderNetworkGraph(networks []docker.NetworkInfo, states map[string]docker.ContainerState) string
```

새로운 `containerLabel()` 헬퍼가 상태에 따라 색상과 아이콘 적용:

```
● 초록    running (실행 중)
◑ 노랑    restarting (재시작 중)
✗ 빨강    dead (종료)
○ 회색    unknown (이벤트 데이터 없음)
```

#### `internal/tui/view.go`

`renderNetworks()`를 lipgloss 사이드-바이-사이드 패널 분할 레이아웃으로 재구현:

- **왼쪽** — `ui.RenderNetworkGraph`에 `m.ContainerStates` 전달
- **오른쪽** — 선택한 네트워크의 컨테이너 이벤트 필터링 타임라인, `die` 이벤트에 `exit=N`, `OOM` 어노테이션 표시

너비 가드: `m.width < 80`이면 `renderNetworksFallback()`(평면 목록)으로 폴백. 좁은 SSH 터미널이나 첫 번째 `WindowSizeMsg` 수신 전 레이아웃 깨짐을 방지합니다.

#### `internal/docker/demo.go`

`StreamEvents`의 die 이벤트에 현실적인 exit code (0, 1, 137)와 SIGKILL 일부에 `OOMKilled=true`를 랜덤 부여. 데모 타임라인에서 장애 전파를 실제처럼 확인 가능합니다.

---

## 트러블슈팅 기록

### CRLF 개행 문자로 Edit 도구 실패

**문제:** 저장소가 Windows에서 `autocrlf=true`로 클론됨. 이전 세션에서 수정한 파일들이 CRLF(`\r\n`) 개행을 가짐. Edit 도구는 바이트 수준으로 매칭하므로 LF 전용 패턴으로는 "String to replace not found" 오류 발생.

**해결:** 파일을 바이트 단위로 읽어 CRLF 패턴을 명시적으로 매칭하고 수정 결과를 다시 쓰는 Go 스크립트 작성:

```go
old := "\t\t\t\tselect {\r\n\t\t\t\tcase ch <- EventInfo{..."
content = strings.Replace(content, old, new, 1)
os.WriteFile("internal/docker/demo.go", []byte(content), 0644)
```

**예방책:** CRLF 파일 수정 시 매칭 문자열에 `\r\n` 포함, 또는 `git config core.autocrlf false` 후 LF로 변환.

### `go mod tidy`가 go 버전 지시자 롤백

**문제:** `go 1.25.0`을 `go 1.23`으로 낮추려 했으나 `go mod tidy`가 `go 1.25.0`으로 원복. 로컬 툴체인이 Go 1.26.1이라 `go 1.25.0` 지시자를 지원하기 때문.

**해결:** 그대로 유지. `go 1.25.0`은 현재 툴체인에서 유효하고 올바른 값.

### `ContainerState` 임포트 사이클 리스크

**문제:** `internal/ui/graph.go`가 토폴로지 노드 색상화를 위해 `ContainerState`를 받아야 함. `internal/tui`에 정의하면 `internal/tui`가 이미 `internal/ui`를 임포트하므로 사이클 발생.

**해결:** `internal/docker`에 정의. `tui`와 `ui` 모두 이미 임포트하는 유일한 패키지. 새로운 임포트 엣지 불필요.

---

## 변경된 파일 목록

| 파일 | 변경 내용 |
|------|---------|
| `internal/docker/state.go` | 신규 — `ContainerState` 구조체 |
| `internal/docker/events.go` | `EventInfo`에 `ExitCode`, `OOMKilled` 필드 추가 |
| `internal/docker/demo.go` | die 이벤트에 현실적인 exit code 추가 |
| `internal/tui/model.go` | `ContainerStates` 맵 필드 |
| `internal/tui/update.go` | `eventMsg` 핸들러에서 상태 전이 처리 |
| `internal/ui/graph.go` | `containerLabel()` + `RenderNetworkGraph` 시그니처 변경 |
| `internal/tui/view.go` | Networks 탭 분할 레이아웃 |

## 릴리즈 정보

- **태그:** `v0.3.0`
- **PR:** [#4](https://github.com/0206pdh/dockviz-cli/pull/4)
- **GitHub 릴리즈:** https://github.com/0206pdh/dockviz-cli/releases/tag/v0.3.0
