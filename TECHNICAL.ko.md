# dockviz-cli 기술 명세

> 제품 범위, 아키텍처, 데이터 흐름, 판정 규칙, 배포 구조를 현재 소스 코드 기준으로 정리한 기술 문서

사용법은 [README.ko.md](README.ko.md), 패널별 사용자 동작은
[docs/reference.ko.md](docs/reference.ko.md)에 있습니다. 이 문서는
"왜 이렇게 만들었는가"와 "코드를 바꿀 때 무엇을 지켜야 하는가"를 다룹니다.
구현이 바뀌면 이 문서도 같은 PR에서 갱신합니다.

기준: v0.8.0 이후 `main`, Go 1.25(`go.mod`), 2026-11-26 갱신.

---

## 0. 한눈에 보기

`dockviz`는 Docker daemon에 직접 연결해 두 가지 운영 질문에 답하는 TUI입니다.

1. 지금 컨테이너에서 의미 있는 장애 신호가 나오고 있는가?
2. Docker 디스크를 무엇이 쓰고 있으며, 무엇을 안전하게 회수할 수 있는가?

```text
입력                          출력
─────────────────────────────────────────────────────────────
ContainerList + Inspect       컨테이너 목록, 포트, 볼륨, CPU/MEM limit
ContainerStats (one-shot)     CPU%, MEM, 최근 60개 history, p95/trend
Events stream                 OOM, 비정상 종료, kill, restart loop
ContainerLogs stream          마지막 50줄 + 실시간 로그
ImageList                     태그별 이미지 목록, 안전한 태그 삭제
system/df + 로그 파일 stat     저장공간 분석, 카테고리별 prune
compose 파일 (선택)            service 의존성, blast radius
VHDX 파일 (Windows)           Docker Desktop host storage gap
```

### 범위

| 포함 | 제외 (의도적으로 지원 종료) |
|---|---|
| Containers / Images / Problems / Disk Usage 패널 | Networks 토폴로지, Events 타임라인 |
| 컨테이너 상세, 로그, CPU/MEM 차트 | `exec` 셸, start/stop/restart |
| 확인창을 거친 삭제와 prune | 이미지 pull 진행률 |
| 로컬·원격 endpoint, `--demo`, Compose context | Compose 파일 편집, Kubernetes/Swarm, 장기 메트릭 저장, 다중 daemon, 웹 UI |

지원이 끝난 기능의 문서는 정리했으며 git 히스토리에 남아 있습니다.

### 하지 않을 것

- `exec`, start/stop/restart 같은 범용 컨테이너 조작을 다시 넣지 않습니다.
- CPU/MEM이 높다는 이유만으로 자동 kill/restart하지 않습니다. 권장 조치는 read-only입니다.
- 단일 시점 `docker stats` 복제에 머무르지 않고, 짧은 history로 "지속적으로 높은지"를 판단합니다.
- Docker Desktop VHDX compaction처럼 host 상태에 따라 위험한 작업은 자동 실행하지 않습니다.

---

## 1. 언어와 라이브러리 선택

### 왜 Go인가

- **단일 바이너리:** 운영 서버에 필요한 것은 `dockviz` 실행 파일 하나입니다.
  SSH로만 접근하는 서버에서 `curl` 한 줄로 설치할 수 있습니다.
- **크로스 컴파일:** `GOOS`/`GOARCH`만 바꿔 6개 플랫폼 바이너리를 만듭니다.
  release workflow가 matrix로 자동화합니다.
- **goroutine과 channel:** 화면 렌더링, 2초 polling, 병렬 stats, event stream,
  log stream이 동시에 돌아야 합니다. 이 프로젝트의 모든 비동기 경로가 이 모델 위에
  있습니다.
- **공식 Docker SDK:** Docker 자체가 Go로 작성되어 있고 SDK가 API 버전 협상
  (`client.WithAPIVersionNegotiation()`)을 처리합니다.

언어가 품질을 보장하지는 않습니다. stream 종료와 재연결, context 수명, CPU delta
계산, ANSI 문자열 폭, 순서가 뒤바뀌는 비동기 응답은 코드가 직접 책임집니다.

### 의존성

| 의존성 | 용도 | 선택 이유 |
|---|---|---|
| `charmbracelet/bubbletea` | TUI 이벤트 루프 | TEA 패턴: 상태 변경 경로가 `Update` 하나 |
| `charmbracelet/lipgloss` | 터미널 스타일 | 선언적 스타일, 색상 팔레트를 `ui/styles.go`에 집중 |
| `charmbracelet/bubbles` | spinner, key binding | Bubble Tea 공식 컴포넌트 |
| `docker/docker` | Docker Engine API | 공식 SDK, 버전 협상, `stdcopy` |
| `compose-spec/compose-go/v2` | Compose 파일 해석 | `docker compose`를 shell out하지 않고 in-process로 해석 |
| `spf13/cobra` | CLI 플래그와 help | 플래그 파싱, `--version` 자동 제공 |

직접 의존성 6개이며 간접 의존성은 `go.sum`이 고정합니다.

### Bubble Tea 요약

```text
Model  — 앱의 전체 상태 (구조체 하나)
Update — 메시지를 받아 새 Model과 다음 Cmd를 반환
View   — Model을 읽어 문자열을 반환 (I/O 없음)
```

`Update`는 I/O를 직접 하지 않고 `tea.Cmd`를 반환합니다. 런타임이 goroutine에서
Cmd를 실행하고 결과 메시지를 다시 `Update`로 보냅니다. 따라서 "상태가 어디서
바뀌었나"의 답은 항상 `Update`입니다.

---

## 2. 아키텍처

### 패키지 구조

```text
main.go                 version 변수(-ldflags 주입)만 두는 최소 진입점
cmd/root.go             Cobra 플래그: --demo, --host, --context, -f/--compose-file, --version
internal/tui/           Model·Update·View, 문제 판정, 비동기 command
internal/docker/        Docker SDK 접근과 도메인 타입 변환, DemoClient
internal/compose/       Compose 파일 탐색·해석
internal/ui/            순수 렌더링 유틸리티 (색상, 상태 아이콘, sparkline)
scenarios/              실제 daemon 부하 시나리오와 stats 벤치마크
packaging/              .deb, APT 저장소 빌드 스크립트
python_pkg/, setup.py   Go 바이너리를 감싸는 Python wheel
```

의존 방향은 `cmd → tui → {docker, compose, ui}`입니다. `docker`와 `compose`는
TUI를 모르고, `ui`는 다른 내부 패키지에 의존하지 않습니다.

### 시작 흐름

```text
cmd.Execute(version)
  → tui.Start(opts)
      ├─ --demo ? docker.NewDemoClient() : docker.NewClient(host)   // Ping 실패 시 종료
      ├─ compose.Load(ctx, "", opts.ComposeFiles)                    // 파일이 없으면 nil context
      ├─ newModel(...)                                               // event stream 즉시 시작
      └─ tea.NewProgram(m, tea.WithAltScreen()).Run()
```

`tea.WithAltScreen()`으로 실행하므로 셸의 scrollback을 오염시키지 않습니다.

### `DockerClient` 인터페이스

TUI는 `internal/docker/interface.go`의 인터페이스에만 의존합니다.

```go
type DockerClient interface {
    ListContainers() ([]ContainerInfo, error)
    ListImages() ([]ImageInfo, error)
    FetchStats(id string) (cpu float64, memMB float64, err error)
    RemoveContainer(id string) error
    RemoveImage(id string) error
    DiskUsage() (DiskUsageInfo, error)
    PruneImages() (freedMB float64, err error)
    PruneContainers() (freedMB float64, err error)
    PruneVolumes() (freedMB float64, err error)
    PruneBuildCache() (freedMB float64, err error)
    PruneLogs() (freedMB float64, err error)
    StreamLogs(ctx context.Context, id string) <-chan LogLine
    StreamEvents(ctx context.Context) <-chan EventInfo
    Close()
}
```

실제 `Client`와 `DemoClient`가 같은 계약을 구현하므로 진입점에서 구현체만 바꾸면
되고, TUI와 판정 로직은 daemon 없이 테스트할 수 있습니다. 대부분의 메서드가
`context.Context`를 받지 않는 것은 알려진 한계입니다(TODOS T-006).

---

## 3. Model과 메시지

### Model이 관리하는 상태

| 그룹 | 필드 | 설명 |
|---|---|---|
| 연결 | `docker`, `demo`, `composeContext` | 사용 중인 client와 선택적 Compose context |
| 데이터 | `containers`, `images` | 마지막으로 반영된 조회 결과 |
| 갱신 순서 | `fetchSeq`, `appliedSeq` | 마지막으로 보낸 조회 번호, 마지막으로 반영한 조회 번호 (§4) |
| history | `history`, `memHistory` | 컨테이너 ID별 CPU%/MEM MB, 최대 60개 |
| Disk Usage | `diskUsage`, `diskUsageLoaded`, `diskUsageErr`, `pruning`, `pruneResultMsg` | 저장공간 분석과 prune 진행 상태 |
| 탐색 | `activePanel`, `activeView`, `cursor`, `selectedID` | 패널(4개)과 화면(Dashboard/Detail/Logs/Chart) |
| 삭제 확인 | `confirmDelete`, `pendingDeleteID` | 확인창을 연 순간 캡처한 대상 ID (§9) |
| 로그 | `logs`, `logScroll`, `logCh`, `logCancel` | 현재 로그 stream |
| 이벤트 | `events`, `eventCh`, `eventCancel`, `eventDisconnected` | 최신순 최대 100개, 끊김 여부 |

### 주요 메시지

| 메시지 | 발생 | 처리 |
|---|---|---|
| `tickMsg` | 2초마다 | 이전 조회가 진행 중이 아니면 새 조회 시작, 다음 tick 예약 |
| `dataMsg{seq, ...}` | 조회 완료 | 오래된 결과는 버리고, 최신이면 목록과 history 반영 |
| `diskUsageRefreshMsg` | Disk Usage/Problems가 보일 때 10초마다 | `system/df` 재조회 |
| `diskUsageMsg` | 저장공간 조회 완료 | 반영. 실패해도 대시보드를 비우지 않음 |
| `pruneDoneMsg` | prune 완료 | 회수 용량 표시 후 재조회 |
| `eventMsg` | event 도착 | 최신순 누적, 다음 event 대기 |
| `logLineMsg` | 로그 한 줄 도착 | 누적, 자동 스크롤, 다음 줄 대기 |

`Init()`은 spinner, 첫 조회(#1), 첫 tick, 첫 event 대기를 한 번에 시작합니다.

---

## 4. 데이터 갱신

### 2초 polling과 병렬 stats

`fetchDataCmd`는 컨테이너 목록과 이미지 목록을 goroutine 두 개로 동시에 가져온
뒤, 실행 중인 컨테이너마다 goroutine을 띄워 stats를 병렬로 받습니다. 순차 조회
대비 12개 컨테이너에서 `19.730s → 2.058s`(9.585배)로 줄었습니다
([검증 기록](docs/validation.ko.md#61-stats-병렬화)).

### 조회 순번: 겹침과 역순 도착 방지

one-shot stats는 daemon이 CPU delta를 계산하려고 두 번 측정하므로 컨테이너가
많으면 조회 한 번이 2초 tick보다 길어질 수 있습니다. 그대로 두면 조회가 겹치고,
늦게 끝난 옛 결과가 최신 결과를 덮어써 history 순서가 뒤틀립니다.

```text
조회를 시작할 때      fetchSeq++ 후 그 번호를 dataMsg.seq에 실어 보냄
dataMsg 도착          msg.seq <= appliedSeq 이면 버림 (이미 더 최신을 반영함)
                      아니면 반영하고 appliedSeq = msg.seq
tick 도착             fetchSeq > appliedSeq (진행 중) 이면 조회를 건너뛰고 tick만 예약
```

수동 새로고침(`r`)과 삭제 후 재조회도 같은 번호 체계를 씁니다. 관련 테스트는
`internal/tui/update_test.go`에 있습니다.

### 실패한 stats 샘플

stats 조회 하나가 실패해도 갱신 전체를 실패시키지 않습니다. 대신 성공한 샘플에만
`ContainerInfo.StatsOK = true`를 표시하고, `StatsOK`가 false인 샘플은 history에
넣지 않습니다. 화면에는 `0%`가 아니라 `-`로 보입니다. 실패를 0으로 기록하면
p95, trend, 메모리 증가 판정이 왜곡되기 때문입니다.

### timeout

갱신 경로의 daemon 호출에는 `internal/docker/client.go`에 정의한 timeout을 겁니다.

| 호출 | timeout |
|---|---|
| `ContainerList`, `ImageList` | 10초 |
| `ContainerStats` (one-shot) | 10초 |
| `ContainerInspect` (limit 조회) | 5초 |

진행 중이면 tick을 건너뛰므로, timeout이 없으면 응답 없는 호출 하나가 갱신을 영원히
멈춥니다. 사용자가 실행한 prune/remove는 원래 오래 걸릴 수 있어 timeout을 걸지
않았습니다.

### CPU 계산

```go
cpuDelta := CPUStats.CPUUsage.TotalUsage - PreCPUStats.CPUUsage.TotalUsage
sysDelta := CPUStats.SystemUsage        - PreCPUStats.SystemUsage
numCPU   := OnlineCPUs (0이면 len(PercpuUsage))
cpu%     := sysDelta == 0 ? 0 : cpuDelta / sysDelta * numCPU * 100
```

CPU%는 코어 하나를 100%로 보는 Docker CLI와 같은 기준이며, 여러 코어를 쓰면
100%를 넘을 수 있습니다.

### resource limit 캐시

CPU/MEM limit은 `ContainerInspect`로 얻고 컨테이너 ID별로 캐시합니다. 목록에서
사라진 ID는 캐시에서 지웁니다. CPU limit은 `NanoCPUs`, `CPUQuota/CPUPeriod`,
`CPUCount` 순으로 해석합니다.

### 저장공간 조회 주기

`system/df`는 비싼 호출이라 2초 tick에 넣지 않습니다. Disk Usage 또는 Problems
패널이 보일 때만 10초마다 조회합니다. 따라서 Problems의 storage 신호는 두 패널 중
하나를 한 번 연 뒤부터 나타납니다.

---

## 5. 실시간 stream

로그와 이벤트는 같은 패턴을 씁니다.

```text
1. client 메서드가 goroutine을 띄우고 receive-only channel을 반환
2. wait Cmd가 channel에서 한 건을 기다렸다가 메시지로 반환
3. Update가 메시지를 반영하고 같은 wait Cmd를 다시 예약
4. context를 취소하면 goroutine이 끝나고 channel이 닫힘 → wait Cmd는 nil 반환
```

### Events

- `type=container` 필터와 `Since = 지금 - 1시간`으로 시작해, 앱을 열자마자 최근
  사건이 반영됩니다.
- `die` event에서 `exitCode`와 `oomKilled` 속성을 읽습니다.
- Model은 최신순으로 최대 100개를 보관합니다.
- 오류로 stream이 끊기면 `Disconnected` sentinel을 보냅니다. Problems에
  `Daemon disconnected`가 뜨고, `r`을 누르면 기존 context를 취소하고 새 stream을
  엽니다.
- event stream은 `newModel()`에서 바로 시작하므로 어느 패널에 있든 수집됩니다.

### Logs

- `Follow: true`, `Tail: "50"`으로 마지막 50줄과 이후 로그를 받습니다.
- non-TTY 컨테이너의 로그는 프레임마다 8바이트 헤더가 붙는 multiplex 형식이라,
  `stdcopy.StdCopy`로 stdout/stderr를 풀어낸 뒤 `bufio.Scanner`로 줄을 나눕니다.
- 새 로그를 열기 전에 이전 log context를 취소하고, `Esc`로 나갈 때도 취소합니다.

---

## 6. 리소스 요약과 문제 판정

판정 로직은 `internal/tui/resources.go`, `projects.go`, `problems.go`에 있고 모두
Model을 읽기만 하는 순수 함수입니다. 사용자용 기준표는
[레퍼런스](docs/reference.ko.md)에 있고, 여기서는 계산 방식을 적습니다.

### 리소스 요약

| 값 | 계산 |
|---|---|
| Current | history 마지막 값 |
| Avg | history 평균 |
| P95 | 정렬 후 `ceil(0.95 × n) - 1` 위치 값 (nearest-rank) |
| Peak | history 최댓값 |
| Trend | 앞 1/3 평균과 뒤 1/3 평균 비교. 차이가 `max(1, 앞 평균의 절댓값 × 10%)`를 넘으면 up/down, 아니면 flat. 샘플 3개 미만은 flat |

### event 기반 판정

최근 10분 event를 오래된 것부터 재생합니다.

| 조건 | 결과 |
|---|---|
| `die` + `oomKilled` | critical `OOM killed` |
| `die` + exit code ≠ 0 | critical `Abnormal exit` |
| `kill` | warning `Killed` |
| 10분 안 `restart` 3회 이상 | `Restart loop` |
| 이후 `start` / `unpause` | 해당 컨테이너의 crash/kill 문제 해제 |
| event stream 끊김 | critical `Daemon disconnected` |

### history 기반 판정 (실행 중 컨테이너)

| 신호 | 조건 |
|---|---|
| CPU | 샘플 3개 이상, 최근 5개 평균 ≥ 60 / 80 / 95% → info / warning / critical |
| Memory pressure | memory limit이 있을 때 `max(current, p95) / limit` ≥ 0.6 / 0.8 / 1.0 |
| Memory growth | 샘플 6개 이상, trend up, 처음 대비 증가량 ≥ `max(50MB, 처음 값 × 20%)`. limit의 90% 이상이면 critical, 아니면 warning |
| No resource limits | limit 조회 성공, CPU·MEM limit 모두 없음. CPU p95 ≥ 60% 또는 MEM p95 ≥ 256MB면 warning, 아니면 info |
| Idle memory | CPU p95 ≤ 5%, MEM p95 ≥ 512MB → info |

### project 기반 판정

Compose project(label이 없으면 `(standalone)`)별로 실행 중 컨테이너 CPU를
합산합니다. 실행 중 컨테이너 2개 이상, project CPU 합 ≥ 50%, 최상위 컨테이너
점유율 ≥ 70%이면 `Noisy neighbor`입니다. project CPU ≥ 100% 또는 최상위 컨테이너
≥ 80%면 warning, 아니면 info입니다.

### storage 기반 판정

저장공간을 한 번이라도 조회한 뒤에만 평가합니다. 카테고리별 reclaimable 값에
기준표의 info/warning/critical 경계를 적용하고, Windows Docker Desktop에서는
VHDX 할당량 중 `system/df` 밖의 차이(host storage gap)도 평가합니다.

문제 목록은 severity 순(critical → warning → info)으로 정렬해 표시합니다.

---

## 7. Compose context

`internal/compose/context.go`가 `compose-go`로 파일을 해석합니다.

- `-f`가 없으면 현재 디렉터리에서 `compose.yaml`, `compose.yml`,
  `docker-compose.yaml`, `docker-compose.yml`을 찾고, 찾은 파일을 모두 합쳐
  해석합니다. 하나도 없으면 nil context로 계속 동작합니다.
- `-f`로 지정한 파일이 없으면 시작 단계에서 오류를 냅니다.
- service마다 image, `depends_on`, 역방향 dependent, network, volume, port, CPU,
  memory limit, restart 정책을 `ServiceContext`로 만듭니다.
- 실행 중 컨테이너는 `com.docker.compose.project` / `com.docker.compose.service`
  label로 service와 연결합니다. label이 기본 정보원이고, Compose 파일은 설명을
  보강하는 read-only 정보입니다.

---

## 8. Disk Usage와 정리

### 불변식: 보여준 숫자 = 실제 회수량

`internal/docker/diskusage.go`의 각 카테고리가 표시하는 `ReclaimMB`는 그 카테고리의
`Prune*` 메서드가 실제로 회수하는 양과 같아야 합니다. 사용자가 화면 숫자를 믿고
눌렀는데 덜 회수되는 상황을 만들지 않기 위해서입니다.

| 카테고리 | prune 방식 | 주의점 |
|---|---|---|
| Images | `ImagesPrune(dangling=true)` | `docker image prune -a`가 아님. 태그가 남은 미사용 이미지는 `OutsidePruneMB`로 따로 표시 |
| Containers | `ContainersPrune()` | created/exited/dead만 대상. running/paused는 Active |
| Volumes | `VolumesPrune(all=true)` | API 1.42+는 필터 없는 prune을 익명 볼륨으로 제한하므로 `all=true`가 필수. 이 불변식이 깨졌던 기록: [버그 기록 7번](docs/troubleshooting.ko.md) |
| Build Cache | `BuildCachePrune(All: true)` | 사용되지 않는 layer만 |
| Logs | 로컬 파일 작업 | 활성 로그는 `os.Truncate`(daemon이 fd를 계속 열고 있어 삭제하면 공간이 안 풀림), 회전된 `<LogPath>.N`은 `os.Remove`. 컨테이너는 건드리지 않음 |

### 측정 불가와 0의 구분

- `Unknown`: 크기나 회수 가능 여부를 측정할 수 없는 객체 수. 0과 섞지 않습니다.
- `Unavailable`: 카테고리 전체를 측정할 수 없는 이유. 예를 들어 로그 파일에서
  `os.IsPermission`이 나오면 `"permission denied on N container(s) — try sudo"`를
  채웁니다. `Glob` 결과로 권한을 추론하지 않는 이유는, 디렉터리를 못 읽어도 오류
  없이 0개를 돌려주기 때문입니다.

### Host Storage (Windows 전용)

`hoststorage_windows.go`(build tag `windows`)가 `%LOCALAPPDATA%`와
`%USERPROFILE%` 아래의 `docker_data.vhdx` / `ext4.vhdx`를 찾아 실제 할당 크기와
host 여유 공간을 잽니다. 원격 `DOCKER_HOST`에서는 측정하지 않습니다. 다른 OS에서는
`hoststorage_other.go`가 unavailable을 돌려줍니다. 이 값은 prune 표와 분리된 진단
정보이며 회수 예상량이 아닙니다.

---

## 9. 사용자 조작의 안전성

### 삭제 대상은 cursor가 아니라 ID로 캡처

확인창이 떠 있는 동안에도 2초 갱신이 돌기 때문에 목록 순서가 바뀔 수 있습니다.
확인 시점에 cursor를 다시 읽으면 다른 항목을 지울 수 있으므로, `d`를 누르는 순간
대상 ID(컨테이너 ID, 이미지 태그, 디스크 카테고리 키)를 `pendingDeleteID`에
저장하고 `y`를 누르면 그 값을 씁니다.

### 삭제 강도

- 컨테이너 삭제는 `Force: true`(`docker rm -f`와 같음).
- 이미지 삭제는 `Force: false`. 태그가 여러 개면 해당 태그만 지워지고 이미지는
  남는다는 안내를 확인창에 표시합니다.
- Local Volumes prune은 데이터 손실 가능성을 확인창에서 따로 경고합니다.

---

## 10. 오류, 취소, 리소스 수명

```text
NewClient / Ping 실패      → tui.Start 오류 → stderr 출력, exit 1
목록 조회 실패             → dataMsg.err → 오류 화면 (다음 성공 시 복구)
개별 stats 실패            → 목록 유지, StatsOK=false, history 제외
system/df 실패             → diskUsageErr 표시, 대시보드 유지
event stream 오류          → Disconnected sentinel → [r]로 재연결
log stream 종료            → channel close → 대기 Cmd 종료
```

- 종료(`q`)할 때 log/event context를 모두 취소합니다.
- 새 로그를 열기 전과 로그 화면을 나갈 때 log context를 취소합니다.
- event 재연결 시 기존 context를 취소한 뒤 새로 만듭니다.
- 목록이 바뀌면 cursor를 `activeListLen()` 범위로 clamp합니다.

---

## 11. 렌더링 규칙

- **패딩 먼저, 색상 나중:** ANSI 코드가 들어간 문자열은 `len()`이 화면 폭보다
  크므로 `fmt.Sprintf("%-12s", ...)`는 반드시 평문에 먼저 적용하고 그 다음
  Lip Gloss로 색을 입힙니다.
- **고정 scale sparkline:** `ui.Sparkline()`은 최근 10개 값을 `▁▂▃▄▅▆▇█`로
  바꾸며, 컨테이너 간 최댓값이 아니라 0~100% 고정 기준을 씁니다. 다른 컨테이너
  부하 때문에 막대가 바뀌지 않습니다. 항상 10 rune 폭입니다.
- **View는 읽기만:** View에서 Docker API를 부르지 않습니다.

---

## 12. 테스트와 CI

```bash
go test ./...
go vet ./...
go run . --demo
```

| 테스트 파일 | 검증 대상 |
|---|---|
| `internal/docker/containers_test.go` | CPU delta, OnlineCPUs fallback, zero system delta, 포트 포맷/중복 제거 |
| `internal/docker/endpoint_test.go` | daemon endpoint 우선순위(`--context`/`--host`/`DOCKER_HOST`/`DOCKER_CONTEXT`/현재 context), context 디렉터리 해시, TLS 파일, 잘못된 설정 오류, context가 `DOCKER_HOST`에 덮이지 않음 |
| `internal/docker/leftovers_test.go` | 잔존 리소스 판정: 실제 Docker Desktop 응답(`testdata/leftovers-desktop-4.100.1.json`) 기반 판정, Compose 파일 유무, 규칙별 경계(실행 중 one-off, 다른 프로젝트, 붙어 있는 볼륨, 기준 일수), 정렬, buildx 이름, demo 데이터 |
| `internal/tui/leftovers_test.go` | Leftovers 행/목록 화면 렌더링, Enter/Esc/이동/`d` 처리, 데이터 반영과 실패 시 이전 목록 유지, Problems 요약, Compose 볼륨 prune 경고, compose scope 변환 |
| `internal/docker/diskusage_test.go` | dangling 판정, 크기 변환/포맷, 로컬 host 판정, VHDX 경로, 로그 파일 크기, demo prune |
| `internal/compose/context_test.go` | Compose 해석, 의존성/역의존성, 지정 파일 없음, 자동 탐색 실패 |
| `internal/tui/problems_test.go` | event/리소스/project/storage 판정 |
| `internal/tui/resources_test.go`, `projects_test.go` | p95, trend, project 합산 |
| `internal/tui/update_test.go` | cursor clamp, 디스크 행, 오래된 결과 무시, 실패 샘플 제외, 진행 중 tick 건너뛰기 |
| `internal/tui/view_test.go` | 0/측정 불가/prune 밖 용량 구분, host storage 분리, 컨테이너·상세·문제 상세 렌더링 |
| `internal/ui/styles_test.go` | sparkline 폭과 고정 scale |

### CI

`.github/workflows/ci.yml`이 모든 PR과 `main` push에서 실행됩니다.

| job | 내용 |
|---|---|
| lint | gofmt 검사, `go mod tidy` diff 검사, `go vet` (linux/windows/darwin) |
| test | ubuntu/macOS/Windows에서 `go build`, `go test` (ubuntu는 `-race`) |

Dependabot(`.github/dependabot.yml`)이 Go 모듈과 GitHub Actions 버전을 매주
확인합니다.

### 실제 daemon 검증

실제 `Client` 메서드는 단위 테스트로 거의 다루지 못합니다(TODOS T-007). 대신
[`scenarios/`](scenarios/)의 PowerShell 시나리오가 라벨된 컨테이너·볼륨·이미지를
만들어 실제 daemon에서 수치를 기록하고, 결과는
[docs/validation.ko.md](docs/validation.ko.md)에 남깁니다.

---

## 13. 배포와 패키징

### release workflow

`v*` tag push가 `.github/workflows/release.yml`을 실행합니다.

```text
determine_version  tag 형식 검사 (vMAJOR.MINOR.PATCH, -rc.1 같은 접미사 허용)
test               go vet + go test (CI와 같은 검사를 tag 커밋에 다시 실행)
  → build          linux/darwin/windows × amd64/arm64, -s -w, -X main.version=<tag>
      ├─ package_python   바이너리를 플랫폼별 wheel에 포함
      └─ package_deb      linux amd64/arm64 .deb
          → release       바이너리·wheel·deb + SHA256SUMS를 GitHub Release에 업로드
          → publish_pypi       (vars.PYPI_PUBLISHING_ENABLED == 'true', Trusted Publishing)
          → deploy_apt_repo    (APT_GPG_PRIVATE_KEY secret이 있을 때, GitHub Pages)
```

release 본문은 자동 생성하지 않고 `gh release edit`로 직접 작성합니다.

### Python wheel

Python으로 다시 만든 앱이 아니라 Go 바이너리 포장입니다. `setup.py`가 미리 빌드된
바이너리를 `dockviz_cli` 패키지에 복사하고, `console_scripts`가
`dockviz_cli._launcher:main`을 노출합니다. launcher는 Unix에서 `os.execv`,
Windows에서 `subprocess.run`으로 바이너리를 실행합니다.
`DOCKVIZ_WHEEL_PLAT_NAME`으로 플랫폼 tag를 지정합니다.

### Debian과 APT

`packaging/build-deb.sh`가 바이너리를 `/usr/bin/dockviz`, LICENSE를
`/usr/share/doc/dockviz/`에 넣습니다. `build-apt-repo.sh`는 architecture별
`Packages`, `Packages.gz`, `Release`를 만들고, GPG 키가 있으면 `InRelease`,
`Release.gpg`, 공개 keyring을 함께 만듭니다.

---

## 14. 운영 제약

- **권한과 endpoint:** socket 접근 권한이 없거나 daemon이 꺼져 있으면 `Ping`에서
  시작이 실패합니다. 인증 없는 원격 TCP socket은 원격 root 권한과 같습니다.
- **Container Logs:** `LogPath`를 dockviz 프로세스가 직접 stat/truncate하므로
  daemon과 파일시스템을 공유할 때만 정확합니다(네이티브 Linux, `dockerd`를 직접
  실행하는 WSL 배포판). Docker Desktop과 원격 host는 unavailable입니다. 환경별
  매트릭스: [docs/reference.ko.md](docs/reference.ko.md#container-logs-제약).
- **데이터 신선도:** 목록/stats는 2초 polling(컨테이너가 많으면 더 느릴 수 있음),
  저장공간은 10초 polling, logs/events는 stream입니다. 서로 짧은 시간 차이가 있을
  수 있습니다.
- **메모리 상한:** event 100개, 컨테이너별 CPU/MEM history 각 60개입니다. 로그는
  현재 화면에서 받은 줄을 모두 누적합니다.

---

## 15. 설계 결정 요약

| 결정 | 이유 | 결과 |
|---|---|---|
| Go 단일 바이너리 | 서버 배포, SDK, 동시성, 크로스 컴파일 | `curl` 한 줄 설치 |
| Bubble Tea (TEA) | 입력·비동기 응답·렌더링을 단방향으로 통합 | 상태 변경 경로가 `Update` 하나 |
| `DockerClient` interface | 실제/데모/테스트 구현 분리 | daemon 없이 TUI 테스트 |
| polling + stream 혼합 | snapshot과 연속 데이터의 성격이 다름 | 데이터별로 맞는 갱신 방식 |
| 병렬 stats | N개 one-shot 조회의 대기 시간 | 12개 기준 9.6배 단축 |
| 조회 순번 + 진행 중 건너뛰기 | 느린 조회가 겹치고 역순 도착 | history 순서 보장 |
| `StatsOK`로 실패 샘플 제외 | 실패를 0으로 기록하면 판정 왜곡 | p95/trend 정확도 유지 |
| 갱신 경로 timeout | 응답 없는 호출이 갱신을 멈춤 | 최악에도 10초 안에 복구 |
| `ReclaimMB` = 실제 회수량 | 화면 숫자에 대한 신뢰 | 카테고리별 안전 기본값 |
| ID 캡처 삭제 | 확인 중 갱신으로 cursor drift | 오삭제 방지 |
| 태그별 이미지 행 | 다중 태그 삭제의 영향을 설명 | 태그만 지우는 안전 삭제 |
| `stdcopy` 역다중화 | Docker multiplex framing | stdout/stderr 로그 보존 |
| Compose는 read-only overlay | label이 실제 상태의 기준 | 파일 없이도 동작 |
| Go 바이너리 + Python wheel + APT | 설치 경로를 넓힘 | 구현은 Go 하나 |

---

## 16. 기능 추가 체크리스트

1. Docker API가 필요하면 `internal/docker`에 도메인 타입과 `DockerClient` 메서드를
   먼저 정의합니다.
2. 실제 `Client`와 `DemoClient` 모두 구현합니다.
3. 갱신 경로에서 부르는 daemon 호출에는 timeout을 겁니다.
4. blocking I/O는 `Update`에서 직접 하지 않고 `tea.Cmd`로 감쌉니다.
5. channel을 만드는 goroutine은 context 취소와 종료 조건을 함께 설계합니다.
6. 응답은 typed `tea.Msg`로 받고 Model은 `Update`에서만 바꿉니다. 여러 번 겹칠 수
   있는 조회라면 순번으로 오래된 결과를 버립니다.
7. View는 Model을 읽기만 합니다. 색상은 평문 패딩 뒤에 입힙니다.
8. 삭제·prune 같은 파괴적 동작은 확인창과 ID 캡처를 씁니다. 표시하는 회수량은
   실제 동작과 같아야 합니다.
9. daemon 없이 검증할 수 있는 순수 함수와 단위 테스트를 추가합니다.
10. CLI, 키 바인딩, README(영/한), `docs/reference.ko.md`, 이 문서를 같은 PR에서 갱신합니다.
