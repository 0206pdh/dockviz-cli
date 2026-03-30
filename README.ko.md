# dockviz-cli

> Docker 환경을 터미널에서 실시간으로 시각화하는 TUI 대시보드

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-4DA6FF?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/0206pdh/dockviz-cli?style=flat-square)](https://github.com/0206pdh/dockviz-cli/releases/latest)

---

## 만들게 된 이유

Docker를 쓰다 보면 컨테이너 상태를 확인하기 위해 아래 명령어들을 반복해서 입력하게 됩니다.

```bash
docker ps
docker stats
docker logs -f nginx
docker network ls
docker images
```

명령어 하나하나는 간단하지만, **여러 컨테이너를 동시에 운영할 때는** 창을 여러 개 열거나 명령어를 반복해야 합니다.  
특히 다음 상황에서 불편함을 느꼈습니다.

- 컨테이너가 5개 이상 돌아갈 때, 어떤 게 CPU를 많이 먹는지 한눈에 파악하기 어려움
- `docker logs`는 실행하면 고정되어 다른 작업을 할 수 없음
- 어떤 컨테이너들이 같은 네트워크에 연결되어 있는지 구조를 파악하기 어려움
- `docker pull`은 레이어 다운로드 진행 상황이 터미널에 텍스트로만 쌓임

`dockviz-cli`는 이 모든 정보를 **하나의 터미널 화면에서 실시간으로** 보여주기 위해 만들었습니다.

---

## 이 도구를 써야 하는 이유

| 기존 방식 | dockviz-cli |
|-----------|-------------|
| `docker ps` + `docker stats`를 번갈아 실행 | 컨테이너 목록과 CPU/MEM을 한 화면에서 실시간 확인 |
| 여러 터미널 창을 열어 각각 모니터링 | Tab 전환으로 컨테이너·네트워크·이미지·이벤트를 한 곳에서 |
| `docker logs -f`를 별도 창에서 실행 | `l` 키 한 번으로 실시간 로그 스트리밍 |
| `docker rm -f` 명령어를 직접 입력 | `d` 키로 확인 팝업 후 삭제, 멀티 태그 이미지 안전 보호 |
| `docker pull`의 텍스트 출력 | 레이어별 프로그레스 바로 시각화 |
| 컨테이너 장애 발생 시 원인 파악 어려움 | 이벤트 타임라인 + 토폴로지 노드 색상으로 장애 전파 즉시 확인 |

`dockviz-cli`는 **단일 바이너리** 하나로 어느 서버에서든 바로 실행됩니다.

---

## 화면 미리보기

```
┌─ dockviz  v0.2.1  •  5 containers ──────────────────────────────────┐
│                                                                     │
│  📦 Containers   🌐 Networks   🗃  Images                          │
│  ──────────────────────────────────────────────────                 │
│                                                                     │
│       NAME              GRAPH        CPU    MEM     STATUS          │
│  ▶    nginx-proxy       ▁▂▃▄▃▂▁▂▃▄   2.1%   45MB    ● running      │
│       api-server        ▃▄▅▆▇▆▅▄▃▄  18.4%  210MB   ● running      │
│       postgres-db       ▁▁▁▂▁▁▁▂▁▁   0.9%  128MB   ● running      │
│       redis-cache       ▁▁▁▁▁▁▁▁▁▁   0.2%   12MB   ● running      │
│       db-migration      ──────────    -      -      ○ exited       │
│                                                                     │
│  [q] Quit  [Tab] Panel  [↑↓] Navigate  [Enter] Detail  [r] Refresh │
│  [s] Start/Stop  [d] Delete  [l] Logs  [p] Pull                    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 설치

### Linux / macOS — 한 줄 설치 (OS 및 아키텍처 자동 감지)

```bash
curl -sL "https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

Linux (amd64/arm64), macOS (Intel/Apple Silicon) 모두 동일한 명령어로 설치됩니다.

업데이트도 같은 명령어를 다시 실행하면 됩니다.

<details>
<summary>플랫폼별 직접 지정</summary>

```bash
# Linux amd64 (일반 서버/VM)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-linux-amd64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# Linux arm64 (라즈베리 파이, AWS Graviton 등)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-linux-arm64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# macOS Intel
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-darwin-amd64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz

# macOS Apple Silicon (M1/M2/M3)
curl -sL https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-darwin-arm64 \
  -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

</details>

### Windows

[Releases 페이지](https://github.com/0206pdh/dockviz-cli/releases/latest)에서 다운로드:
- `dockviz-windows-amd64.exe` — Intel/AMD
- `dockviz-windows-arm64.exe` — ARM (Surface Pro X 등)

### 소스 빌드

```bash
git clone https://github.com/0206pdh/dockviz-cli.git
cd dockviz-cli
go build -o dockviz .
```

### go install

```bash
go install github.com/0206pdh/dockviz-cli@latest
```

---

## 사용법

```bash
# Docker 데몬에 연결해서 실행
dockviz

# Docker 없이 데모 모드로 실행
dockviz --demo

# 원격 Docker 데몬에 연결
dockviz --host tcp://192.168.1.100:2375

# 또는 표준 Docker 환경변수 사용
DOCKER_HOST=tcp://192.168.1.100:2375 dockviz

# 이미지 레이어별 다운로드 진행 상황 시각화
dockviz pull nginx:alpine

# 버전 확인
dockviz --version
```

---

## 주요 기능 상세

### 1. 실시간 컨테이너 대시보드

2초마다 자동으로 새로고침되며, 각 컨테이너의 CPU 사용률·메모리 사용량·상태·포트를 한눈에 보여줍니다.

### 2. CPU 스파크라인 (▁▂▃▄▅▆▇█)

각 컨테이너 행에 최근 10개 CPU 수치를 유니코드 블록 문자로 표시합니다.
숫자만 보는 것보다 CPU 부하의 **추세**를 직관적으로 파악할 수 있습니다.

### 3. 장애 전파 시각화 (Networks 탭)

Networks 탭은 좌우 분할 레이아웃으로 구성됩니다.

- **왼쪽 — 토폴로지**: 컨테이너를 색상 아이콘으로 표시 (`● 실행중 / ◑ 재시작중 / ✗ 죽음 / ○ 알 수 없음`)
- **오른쪽 — 이벤트 타임라인**: 네트워크별 컨테이너 생명주기 이벤트, 종료 코드, OOM Kill 표시

```
  app-network  : ● nginx-proxy ─── ● api-server ─── ✗ worker
  db-network   : ● api-server  ─── ● postgres-db ─── ● redis-cache
```

컨테이너가 죽으면 토폴로지에서 즉시 빨간색으로 강조 표시되어 장애가 어디서 시작됐는지 한눈에 파악할 수 있습니다.

### 4. 이벤트 타임라인 (Events 탭)

Docker 컨테이너 생명주기 이벤트(create / start / die / restart / destroy)를 실시간으로 스트리밍합니다.
`die` 이벤트에는 종료 코드와 OOM Kill 여부를 함께 표시합니다.

### 5. 이미지 브라우저

태그별로 한 행씩 표시하며, 알파벳 순으로 정렬됩니다.
`d` 키로 삭제 시 태그 하나씩 안전하게 삭제하며, 여러 태그가 있는 이미지는 경고 팝업을 표시합니다.

### 6. 실시간 로그 스트리밍

`l` 키를 누르면 선택한 컨테이너의 로그를 실시간으로 스트리밍합니다.
ERROR는 빨간색, WARN은 노란색으로 자동 색상 처리됩니다.

### 7. 이미지 Pull 진행 시각화

`dockviz pull <이미지>` 명령으로 레이어별 다운로드 진행 상황을 프로그레스 바로 보여줍니다.

```
  Pulling nginx:alpine

  abc1234abc12  ████████████░░░░░░░░  61%   4.2 MB / 6.9 MB   Downloading
  b2c3456b2c34  ████████████████████ 100%                      Pull complete ✓
  c3d4567c3d45  ────────────────────                           Already exists
```

### 8. 컨테이너 제어

| 키 | 동작 |
|----|------|
| `s` | 선택한 컨테이너 시작 / 정지 |
| `d` | 선택한 컨테이너 또는 이미지 태그 삭제 (확인 팝업) |
| `l` | 실시간 로그 스트리밍 |
| `Enter` | 컨테이너 상세 정보 보기 |
| `r` | 강제 새로고침 / 이벤트 스트림 재연결 |

### 9. 데모 모드

`--demo` 플래그를 사용하면 Docker 데몬 없이도 TUI를 체험할 수 있습니다.
CPU/메모리 수치가 물결처럼 변하는 애니메이션 데이터를 보여줍니다.

---

## 키보드 단축키 전체

| 키 | 동작 |
|----|------|
| `q` / `Ctrl+C` | 종료 |
| `Tab` | 패널 전환 (Containers → Networks → Images → Events) |
| `↑` / `k` | 위로 이동 |
| `↓` / `j` | 아래로 이동 |
| `Enter` | 컨테이너 상세 보기 |
| `Esc` | 뒤로 가기 |
| `s` | 선택한 컨테이너 시작 / 정지 |
| `d` | 선택한 컨테이너 또는 이미지 태그 삭제 *(확인 필요)* |
| `l` | 실시간 로그 스트리밍 |
| `r` | 강제 새로고침 / 이벤트 스트림 끊김 시 재연결 |
| `g` | 선택한 컨테이너의 CPU/MEM 히스토리 전체 화면 차트 열기 |

---

## 기술 스택 및 설계

### 사용 라이브러리

| 역할 | 라이브러리 | 선택 이유 |
|------|-----------|----------|
| TUI 프레임워크 | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | Elm Architecture 기반, 비동기 Cmd 모델로 Docker API 호출을 메인 루프 밖에서 처리 |
| TUI 스타일링 | [Lip Gloss](https://github.com/charmbracelet/lipgloss) | CSS와 유사한 선언적 스타일로 색상·테두리·레이아웃 정의 |
| TUI 컴포넌트 | [Bubbles](https://github.com/charmbracelet/bubbles) | 스피너, 키바인딩 등 재사용 가능한 TUI 컴포넌트 |
| Docker 연동 | [Docker SDK for Go](https://pkg.go.dev/github.com/docker/docker/client) | Docker 공식 Go 클라이언트 |
| CLI 프레임워크 | [Cobra](https://github.com/spf13/cobra) | 서브커맨드(`pull`), 플래그(`--demo`, `--version`) 관리 |

### 아키텍처 — The Elm Architecture (TEA)

이 프로젝트는 Bubble Tea가 채택한 **TEA 패턴**을 따릅니다.

```
main.go
  └── cmd.Execute()               ← Cobra CLI (--demo 플래그)
        └── tui.Start()
              ├── docker.NewClient()   또는   docker.NewDemoClient()
              └── tea.NewProgram(model)
                    ├── Init()    → 첫 데이터 요청 + 2초 타이머 + 이벤트 스트림 시작
                    ├── Update()  → 키 입력 · 타이머 · Docker 응답 · 이벤트 처리
                    └── View()    → Lip Gloss로 화면 문자열 생성
```

**TEA 패턴을 선택한 이유**
- 상태(Model) · 로직(Update) · 렌더링(View)이 완전히 분리되어 있어 유지보수가 쉬움
- 모든 상태 변경이 단방향으로 흐르므로 버그 추적이 명확함
- Docker API 호출·로그 스트리밍 같은 비동기 작업이 `Cmd`로 격리되어 UI가 블로킹되지 않음

### 패키지 구조

```
dockviz-cli/
├── main.go                        # 진입점
├── cmd/
│   ├── root.go                    # Cobra CLI, --demo 플래그
│   └── pull.go                    # `dockviz pull <image>` 서브커맨드
└── internal/
    ├── docker/
    │   ├── interface.go           # DockerClient 인터페이스
    │   ├── client.go              # 실제 Docker SDK 래퍼
    │   ├── demo.go                # 데모 데이터 (데몬 불필요)
    │   ├── containers.go          # 목록, 통계, 시작/정지/재시작/삭제
    │   ├── networks.go            # 네트워크 토폴로지 (NetworkInspect)
    │   ├── images.go              # 이미지 목록 (태그별 한 행, 정렬)
    │   ├── state.go               # ContainerState — 이벤트 스트림 기반 건강 상태
    │   ├── events.go              # 생명주기 이벤트 스트리밍 (ExitCode, OOMKilled)
    │   ├── pull.go                # 이미지 Pull + 레이어별 진행 스트림
    │   └── logs.go                # 컨테이너 로그 스트리밍 (stdcopy 역다중화)
    ├── tui/
    │   ├── model.go               # 상태 (TEA Model) — ContainerStates 맵
    │   ├── update.go              # 이벤트 처리 (TEA Update) — 상태 전이
    │   ├── view.go                # 렌더링 (TEA View) — Networks 분할 레이아웃
    │   ├── keymap.go              # 키보드 바인딩
    │   ├── pull.go                # Pull 진행 TUI 프로그램
    │   └── start.go               # Docker 클라이언트 → TUI 연결
    └── ui/
        ├── styles.go              # Lip Gloss 색상 팔레트, 스타일, 스파크라인
        └── graph.go               # 건강 상태 색상 노드가 있는 토폴로지 그래프
```

### DockerClient 인터페이스

실제 Docker 데몬 클라이언트와 데모 클라이언트가 동일한 인터페이스를 구현합니다.  
TUI 코드는 실제 환경인지 데모 환경인지 알 필요가 없습니다.

```go
type DockerClient interface {
    ListContainers() ([]ContainerInfo, error)
    ListNetworks()   ([]NetworkInfo, error)
    ListImages()     ([]ImageInfo, error)
    FetchStats(id string) (cpu float64, memMB float64, err error)
    StartContainer(id string)   error
    StopContainer(id string)    error
    RestartContainer(id string) error
    RemoveContainer(id string)  error
    RemoveImage(id string)      error
    StreamLogs(ctx context.Context, id string) <-chan LogLine
    StreamEvents(ctx context.Context)          <-chan EventInfo
    Close()
}
```

---

## CI/CD — GitHub Actions 자동 릴리즈

```bash
git tag v1.0.0 && git push --tags
```

위 명령어 한 줄이면 GitHub Actions가 자동으로:

1. Linux / Windows / macOS 바이너리를 크로스 컴파일
2. GitHub Releases에 바이너리 업로드
3. 누구든 `curl` 한 줄로 즉시 설치 가능

---

## 개발 로드맵

- [x] 프로젝트 구조 설계 및 빌드 파이프라인
- [x] Docker SDK 래퍼 + DockerClient 인터페이스 (실제 + 데모 모드)
- [x] 컨테이너 목록 패널 (실시간 CPU/MEM 병렬 조회)
- [x] CPU 스파크라인 — 0-100% 고정 스케일, 컨테이너당 10포인트 유니코드 바
- [x] 네트워크 토폴로지 그래프 + 건강 상태 색상 노드 (● ◑ ✗ ○)
- [x] Networks 탭 분할 레이아웃 — 왼쪽 토폴로지, 오른쪽 네트워크별 이벤트 타임라인
- [x] ContainerState 추적 — 이벤트 스트림 기반 장애 전파 시각화
- [x] 컨테이너 생명주기 이벤트 스트리밍 (종료 코드, OOM Kill 감지)
- [x] 이미지 브라우저 — 태그별 한 행, 알파벳 정렬, 태그 단위 안전 삭제
- [x] 컨테이너 상세 보기
- [x] `--demo` 모드 (Docker 없이 체험)
- [x] `dockviz pull` — 레이어별 다운로드 진행 시각화
- [x] 컨테이너 / 이미지 삭제 확인 팝업 (`d` 키)
- [x] 실시간 로그 스트리밍 (색상 코딩, `l` 키)
- [x] 이벤트 스트림 연결 끊김 감지 + `r`로 재연결
- [x] GitHub Actions 릴리즈 파이프라인 (Linux / Windows / macOS 바이너리 자동 빌드)
- [x] 원격 Docker 호스트 지원 (`--host` 플래그 + `DOCKER_HOST` 환경변수)
- [x] CPU/MEM 히스토리 전체 화면 차트 — 컨테이너별 바 차트 (`g` 키)

---

## 라이선스

MIT © 2026 [0206pdh](https://github.com/0206pdh)
