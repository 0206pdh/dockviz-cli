<div align="center">

# dockviz-cli

**Docker 환경을 터미널에서 실시간으로 시각화하는 TUI 대시보드**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-4DA6FF?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/0206pdh/dockviz-cli?style=flat-square)](https://github.com/0206pdh/dockviz-cli/releases/latest)
[![Built with Bubble Tea](https://img.shields.io/badge/TUI-Bubble%20Tea-FF75B7?style=flat-square)](https://github.com/charmbracelet/bubbletea)

**[English Documentation](README.md)**

</div>

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

명령어 하나하나는 간단하지만, 여러 컨테이너를 동시에 운영할 때는 창을 여러 개 열거나 명령어를 반복해야 합니다. 특히 다음 상황에서 불편함을 느꼈습니다.

- 컨테이너가 5개 이상 돌아갈 때 어떤 게 CPU를 많이 먹는지 한눈에 파악하기 어려움
- `docker logs -f`는 실행하면 터미널이 고정되어 다른 작업을 할 수 없음
- 어떤 컨테이너들이 같은 네트워크에 연결되어 있는지 구조를 파악하기 어려움
- `docker pull`은 레이어 다운로드 진행 상황이 터미널에 텍스트로만 쌓임

`dockviz-cli`는 이 모든 정보를 **하나의 터미널 화면에서 실시간으로** 보여주기 위해 만들었습니다.

---

## 기존 방식과 비교

| 기존 방식 | dockviz-cli |
|-----------|-------------|
| `docker ps` + `docker stats`를 번갈아 실행 | 컨테이너 목록과 CPU/MEM을 한 화면에서 실시간 확인 |
| 여러 터미널 창을 열어 각각 모니터링 | Tab 전환으로 컨테이너·네트워크·이미지·이벤트를 한 곳에서 |
| `docker logs -f`를 별도 창에서 실행 | `l` 키 한 번으로 실시간 로그 스트리밍, `Esc`로 닫기 |
| `docker rm -f` 명령어를 직접 입력 | `d` 키로 확인 팝업 후 삭제, 멀티 태그 이미지 안전 보호 |
| `docker pull`의 텍스트 출력 | 레이어별 프로그레스 바로 시각화 |
| 컨테이너 장애 발생 시 원인 파악 어려움 | 이벤트 타임라인 + 토폴로지 노드 색상으로 장애 전파 즉시 확인 |

단일 바이너리 하나로 어느 서버에서든 바로 실행됩니다. 런타임 의존성 없음.

---

## 화면 미리보기

<img width="1297" height="61" alt="image" src="https://github.com/user-attachments/assets/653aa3ee-fdec-4a86-bb3d-e282601678b2" />
<img width="681" height="242" alt="image" src="https://github.com/user-attachments/assets/d08a5c2b-3019-47ee-a721-3c6e1e3c816f" />
<img width="673" height="282" alt="image" src="https://github.com/user-attachments/assets/bd08af97-00af-4ae2-8984-3b1f26540f5c" />
<img width="630" height="397" alt="image" src="https://github.com/user-attachments/assets/fa896233-26af-4747-829a-83d0905e3e1b" />

---

## 설치

### Linux / macOS — 한 줄 설치 (OS 및 아키텍처 자동 감지)

```bash
rm -f /usr/local/bin/dockviz; curl -sL "https://github.com/0206pdh/dockviz-cli/releases/latest/download/dockviz-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o /usr/local/bin/dockviz && chmod +x /usr/local/bin/dockviz
```

Linux (amd64/arm64), macOS (Intel/Apple Silicon) 모두 동일한 명령어로 설치됩니다.
업데이트도 같은 명령어를 다시 실행하면 됩니다.

> **참고:** 앞의 `rm -f`는 기존 바이너리를 먼저 지우기 위한 것입니다. 덮어쓰기만 하면 셸이 이전 inode를 캐시해서 `--version`이 구버전을 출력하는 경우가 있습니다.

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
숫자만 보는 것보다 CPU 부하의 추세(스파이크, 점진적 상승, 유휴 상태)를 직관적으로 파악할 수 있습니다.

### 3. 통계 히스토리 차트 — `g` 키

실행 중인 컨테이너에서 `g`를 누르면 최근 60개 수치(2분 히스토리)를 전체 화면 막대 차트로 볼 수 있습니다.

**CPU%가 100%를 넘는 이유.** Docker의 CPU% 계산 공식은 다음과 같습니다.

```
(컨테이너 CPU 사용 시간 델타) / (시스템 전체 CPU 시간 델타) × 코어 수 × 100
```

CPU 제한을 걸지 않은 컨테이너는 머신의 모든 코어를 사용할 수 있습니다. 4코어 머신에서 4코어를 전부 사용하면 400%가 나옵니다. Y축은 실제 데이터 기준으로 100% 단위로 자동 확장되어, 부하 수준과 무관하게 막대 높이 변화를 항상 볼 수 있습니다.

임계선은 **80%**(빨강)과 **50%**(노랑)을 절댓값 기준으로 표시합니다. 코어 수와 무관하게 단일 코어 포화 기준으로 의미 있는 지표이기 때문입니다.

```
  CPU   187.3%   0 – 200%
  200% ┤ ████████████████████████████████████
       ┤ ████████████████████████████████████
       ┤ ██████████████████████·············
  100% ┤ ████████████████████████████████████
       ┤ ████████████████████████████████████
  80%  ┤·················████████████████████  ← 빨강 임계선
       ┤                 ████████████████████
  50%  ┤·················████████████████████  ← 노랑 임계선
    0  └──────────────────────────────── now
       ← 2m ago
```

막대가 임계선에 아직 도달하지 않은 빈 셀에는 색상 점(`·`)으로 경계선을 표시합니다.

### 4. 장애 전파 시각화 — Networks 탭

Networks 탭은 좌우 분할 레이아웃으로 구성됩니다.

- **왼쪽 — 토폴로지**: 컨테이너를 색상 아이콘으로 연결
  - `●` 초록 — 실행중
  - `◑` 노랑 — 재시작중
  - `✗` 빨강 — 죽음 / 비정상 종료
  - `○` 회색 — 알 수 없음 / 정상 종료
- **오른쪽 — 이벤트 타임라인**: 네트워크별 컨테이너 생명주기 이벤트, 종료 코드, OOM Kill 표시

```
  app-network  : ● nginx-proxy ─── ● api-server ─── ✗ worker
  db-network   : ● api-server  ─── ● postgres-db ─── ● redis-cache
```

컨테이너가 죽으면 토폴로지에서 즉시 빨간색으로 바뀝니다. 장애가 어디서 시작됐는지 한눈에 파악할 수 있습니다.

### 5. 이벤트 타임라인 — Events 탭

Docker 컨테이너 생명주기 이벤트(`create`, `start`, `die`, `restart`, `destroy`)를 실시간으로 스트리밍합니다. `die` 이벤트에는 종료 코드와 OOM Kill 여부를 함께 표시합니다. 스트림이 끊기면 자동 재연결을 시도하며, `r` 키로 강제 재연결도 가능합니다.

### 6. 이미지 브라우저

태그별로 한 행씩 표시하며, 알파벳 순으로 정렬됩니다. `d` 키로 삭제 시 선택한 태그만 제거합니다. 같은 이미지 ID에 태그가 여러 개 있으면 경고 팝업으로 전체 태그 목록을 보여줍니다. 이미지 자체는 모든 태그가 제거될 때까지 유지됩니다.

### 7. 실시간 로그 스트리밍

`l` 키를 누르면 선택한 컨테이너의 로그를 실시간으로 스트리밍합니다. `ERROR`는 빨간색, `WARN`은 노란색으로 자동 색상 처리됩니다. `Esc`로 닫습니다.

### 8. 이미지 Pull 진행 시각화 — `dockviz pull`

```
  Pulling nginx:alpine

  abc1234abc12  ████████████░░░░░░░░  61%   4.2 MB / 6.9 MB   Downloading
  b2c3456b2c34  ████████████████████ 100%                      Pull complete ✓
  c3d4567c3d45  ────────────────────                           Already exists
```

레이어마다 개별 프로그레스 바를 표시합니다. 모든 레이어가 완료될 때 전체 Pull이 끝납니다.

### 9. 데모 모드

`dockviz --demo`는 Docker 데몬 없이 완전히 동작합니다. CPU/메모리 수치가 사인파형으로 애니메이션되어 모든 탭과 키 바인딩을 실제 환경 없이 체험할 수 있습니다.

### 10. 컨테이너 셸 접속 — `e` 키

실행 중인 컨테이너를 선택하고 `e`를 누르면 컨테이너 안으로 인터랙티브 셸이 열립니다. dockviz가 일시 정지되고 터미널이 셸 세션으로 전환됩니다. `exit`로 나오면 dockviz가 다시 시작됩니다.

```
  # dockviz에서 실행 중인 컨테이너 선택 후 e 키
  # 터미널이 다음과 같이 바뀜:
  / # ls /app
  / # ps aux
  / # exit
  # dockviz 재개
```

`/bin/bash`를 먼저 시도하고 없으면 `/bin/sh`로 자동 폴백합니다. Alpine, Debian, Ubuntu 등 POSIX 셸이 있는 모든 이미지에서 동작합니다. `--host`가 설정된 경우 동일한 원격 데몬에 exec가 전달됩니다.

`--demo` 모드에서는 실제 컨테이너가 없으므로 비활성화됩니다.

### 11. 볼륨 마운트 표시

컨테이너 상세 보기(`Enter`)에서 볼륨 마운트 정보를 확인할 수 있습니다.

```
  ID       a1b2c3d4e5f6
  Image    postgres:16
  Status   ● running
  Ports    5432
  Volumes  postgres_data → /var/lib/postgresql/data
           /backup → /backup (ro)
```

명명 볼륨은 볼륨 이름을, 바인드 마운트는 호스트 경로를 표시합니다. 읽기 전용 마운트는 `(ro)`로 표시됩니다.

### 12. 원격 호스트 지원

```bash
dockviz --host tcp://192.168.1.100:2375
# 또는
DOCKER_HOST=tcp://192.168.1.100:2375 dockviz
```

`--host`가 `DOCKER_HOST`보다 우선합니다. `pull` 서브커맨드에도 동일하게 적용됩니다.

---

## 키보드 단축키 전체

| 키 | 동작 |
|----|------|
| `q` / `Ctrl+C` | 종료 |
| `Tab` | 패널 전환 (Containers → Networks → Images → Events) |
| `↑` / `k` | 위로 이동 |
| `↓` / `j` | 아래로 이동 |
| `Enter` | 컨테이너 상세 보기 |
| `Esc` | 뒤로 가기 / 오버레이 닫기 |
| `s` | 선택한 컨테이너 시작 / 정지 |
| `d` | 선택한 컨테이너 또는 이미지 태그 삭제 *(확인 필요)* |
| `l` | 실시간 로그 스트리밍 |
| `r` | 강제 새로고침 / 이벤트 스트림 끊김 시 재연결 |
| `g` | 선택한 컨테이너의 CPU/MEM 히스토리 전체 화면 차트 열기 |
| `e` | 선택한 컨테이너 안에서 인터랙티브 셸 열기 *(실행 중인 컨테이너만)* |

---

## 기술 스택 및 설계

### The Elm Architecture (TEA)

이 프로젝트는 [Bubble Tea](https://github.com/charmbracelet/bubbletea)가 채택한 TEA 패턴을 따릅니다.

```
main.go
  └── cmd.Execute()               ← Cobra CLI (--demo, --host 플래그)
        └── tui.Start()
              ├── docker.NewClient()      ← 실제 Docker SDK 래퍼
              │   또는 docker.NewDemoClient()  ← 사인파형 데모 데이터
              └── tea.NewProgram(model)   ← Bubble Tea 이벤트 루프
                    ├── Init()    → 첫 데이터 요청 + 2초 타이머 + 이벤트 스트림 시작
                    ├── Update()  → 키 입력 · 타이머 · Docker 응답 · 상태 전이
                    └── View()    → Lip Gloss로 화면 문자열 생성
```

**TEA를 선택한 이유**
- 상태(Model), 로직(Update), 렌더링(View)이 완전히 분리되어 유지보수가 쉬움
- 모든 상태 변경이 단방향으로 흐르므로 버그 추적이 명확함
- Docker API 호출, 로그 스트리밍 같은 비동기 작업이 `Cmd`로 격리되어 UI가 블로킹되지 않음

### 패키지 구조

```
dockviz-cli/
├── main.go                        # 진입점 — ldflags로 빌드 타임 버전 주입
├── cmd/
│   ├── root.go                    # Cobra 루트 커맨드 — --demo, --host 플래그
│   └── pull.go                    # `dockviz pull <image>` 서브커맨드
└── internal/
    ├── docker/
    │   ├── interface.go           # DockerClient 인터페이스 (실제 + 데모 공유)
    │   ├── client.go              # 실제 Docker SDK 래퍼 (FromEnv + 선택적 WithHost)
    │   ├── demo.go                # 사인파형 데모 데이터, 데몬 불필요
    │   ├── containers.go          # 목록, 통계 (병렬 조회), 시작/정지/삭제
    │   ├── networks.go            # 토폴로지: NetworkList 후 NetworkInspect per network
    │   ├── images.go              # 이미지 목록 — 태그별 한 행, 알파벳 정렬
    │   ├── state.go               # ContainerState — 이벤트 스트림 기반 건강 상태
    │   ├── events.go              # 생명주기 이벤트 스트리밍 (ExitCode, OOMKilled)
    │   ├── pull.go                # 이미지 Pull + 레이어별 진행 스트림
    │   └── logs.go                # 컨테이너 로그 스트리밍 (stdcopy 역다중화)
    ├── tui/
    │   ├── model.go               # TEA Model — history/memHistory 맵 포함 전체 UI 상태
    │   ├── update.go              # TEA Update — 키 처리, 타이머, Docker 메시지 라우팅
    │   ├── view.go                # TEA View — 모든 패널, 차트, 오버레이 렌더링
    │   ├── keymap.go              # 키 바인딩 (bubbles/key)
    │   ├── pull.go                # 독립적인 Pull 진행 TUI 프로그램
    │   └── start.go               # Docker 클라이언트 → tea.NewProgram 연결
    └── ui/
        ├── styles.go              # Lip Gloss 색상 팔레트, 공유 스타일, 스파크라인
        └── graph.go               # 건강 상태 색상 노드가 있는 토폴로지 그래프 렌더러
```

### DockerClient 인터페이스

실제 Docker 데몬 클라이언트와 데모 클라이언트가 동일한 인터페이스를 구현합니다. TUI 코드는 실제 환경인지 데모 환경인지 알 필요가 없습니다.

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

### Exec 셸 동작 원리

`e` 키는 `DockerClient` 인터페이스를 거치지 않습니다. `tea.ExecProcess`를 통해 시스템 `docker exec -it <name> sh` 프로세스를 직접 실행합니다. Bubble Tea 이벤트 루프가 일시 정지되고 터미널이 서브프로세스에 넘겨집니다. 셸이 종료되면 루프가 재개되고 데이터를 새로 불러옵니다.

```go
cmd := exec.Command("docker", "exec", "-it", containerName, "sh", "-c", "bash 2>/dev/null || sh")
return tea.ExecProcess(cmd, func(err error) tea.Msg {
    return execDoneMsg{err: err}
})
```

### 버전 주입

바이너리 버전은 빌드 시 ldflags로 주입됩니다.

```bash
go build -ldflags="-X main.version=v1.2.3" -o dockviz .
```

`dockviz --version`은 항상 빌드된 태그를 그대로 출력합니다. ldflags 없이 로컬 `go build`를 하면 `dev`로 표시됩니다.

---

## CI/CD — GitHub Actions 자동 릴리즈

버전 태그를 푸시하면 GitHub Actions가 자동으로 빌드하고 릴리즈합니다.

```bash
git tag v1.2.3 && git push origin v1.2.3
```

Actions 동작:
1. Linux / macOS / Windows 6개 타겟을 크로스 컴파일
2. `-ldflags="-X main.version=${{ github.ref_name }}"` 으로 태그 버전 주입
3. GitHub Releases에 바이너리 업로드

설치 명령어의 `/releases/latest/download/` 경로는 GitHub이 항상 최신 릴리즈로 리다이렉트합니다.

---

## 사용 라이브러리

| 역할 | 라이브러리 | 선택 이유 |
|------|-----------|----------|
| TUI 프레임워크 | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | Elm Architecture 기반, 비동기 Cmd 모델로 Docker API 호출을 메인 루프 밖에서 처리 |
| TUI 스타일링 | [Lip Gloss](https://github.com/charmbracelet/lipgloss) | CSS와 유사한 선언적 스타일로 색상·테두리·레이아웃 정의 |
| TUI 컴포넌트 | [Bubbles](https://github.com/charmbracelet/bubbles) | 스피너, 키바인딩 등 재사용 가능한 TUI 컴포넌트 |
| Docker 연동 | [Docker SDK for Go](https://pkg.go.dev/github.com/docker/docker/client) | Docker 공식 Go 클라이언트 |
| CLI 프레임워크 | [Cobra](https://github.com/spf13/cobra) | 서브커맨드(`pull`), 플래그(`--demo`, `--host`, `--version`) 관리 |

---

## 개발 로드맵

- [x] 프로젝트 구조 설계 및 빌드 파이프라인
- [x] Docker SDK 래퍼 + DockerClient 인터페이스 (실제 + 데모 모드)
- [x] 컨테이너 목록 패널 (실시간 CPU/MEM 병렬 조회)
- [x] CPU 스파크라인 — 컨테이너당 10포인트 유니코드 바
- [x] 네트워크 토폴로지 그래프 + 건강 상태 색상 노드 (● ◑ ✗ ○)
- [x] Networks 탭 분할 레이아웃 — 왼쪽 토폴로지, 오른쪽 이벤트 타임라인
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
- [x] 동적 CPU Y축 — 멀티코어 컨테이너에서 100% 초과 시 자동 확장
- [x] 볼륨 마운트 표시 — 컨테이너 상세 보기에서 마운트 경로 확인
- [x] 인터랙티브 exec 셸 — `e` 키로 TUI를 일시 정지하고 컨테이너 셸 접속

---

## 라이선스

MIT © 2026 [0206pdh](https://github.com/0206pdh)
