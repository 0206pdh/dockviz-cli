# TODOS

dockviz의 고도화 로드맵과 남은 작업입니다. 2026-11-26, v0.8.0 기준으로 정리했습니다.
범위와 "하지 않을 것"은 [TECHNICAL.ko.md §0](TECHNICAL.ko.md#0-한눈에-보기)을
따릅니다.

---

## 고도화 방향

지금 dockviz는 "한 화면에서 보고, 안전하게 지운다"까지 해냅니다. 다음 단계는 세
방향입니다.

1. **믿을 수 있게:** 실제 daemon 테스트, 취소 가능한 호출, 큰 환경에서도 안정적인 갱신
2. **원인까지 보이게:** 문제가 떴을 때 "왜"를 같은 화면에서 답하기
3. **TUI 밖에서도 쓰이게:** 스크립트, cron, CI에서 같은 판정을 쓰기

### 1단계: 기반 다지기 (먼저)

고도화 기능이 늘수록 실제 daemon에서 깨지는 버그의 비용이 커집니다. 기능보다 먼저
합니다.

| ID | 항목 | 효과 |
|---|---|---|
| T-007 | `Client` 실제 daemon 통합 테스트 (CI ubuntu) | volume prune 같은 회수량 버그를 머지 전에 잡음 |
| T-006 | context를 받는 `DockerClient` | 종료·화면 전환 시 진행 중 호출 취소, prune/remove에도 제한 |
| T-009 | stats 동시 조회 수 제한 + 마지막 정상값 유지 | 컨테이너 수십 개에서도 daemon burst 방지, 빈 값 대신 직전 값 표시 |
| T-010 | event 기반 즉시 갱신 | `die`/`oom`/`kill` 도착 시 바로 조회해 2초 대기 없이 반영 |

### 2단계: 원인까지 보이는 Problems

지금은 "OOM이 났다"까지 알려주고 원인 확인은 사용자가 로그와 차트를 따로 열어야
합니다. 문제 상세 한 화면에서 판단이 끝나게 만드는 것이 가장 큰 사용자 가치입니다.

| ID | 항목 | 효과 |
|---|---|---|
| T-011 | 문제 상세에 종료 직전 로그 tail | `ContainerLogs(since, until)`로 `die` 시점 전후 N줄 표시. "왜 죽었나"를 바로 확인 |
| T-012 | 문제 상세에 직전 CPU/MEM 추이 | OOM 직전 메모리 곡선을 같은 화면에 표시해 leak과 순간 급증을 구분 |
| T-013 | healthcheck 신호 | `health_status: unhealthy` event와 inspect의 `Health`를 Problems에 반영 (지금은 무시함) |
| T-014 | Compose 영향 전파 표시 | 문제가 난 service에 `depends_on`으로 묶인 service를 함께 표시 |
| T-015 | replica를 service 단위로 합산 | 같은 service replica 여러 개를 한 줄로 묶어 project 요약 정확도 향상 |

### 3단계: TUI 밖으로 (headless 모드)

같은 판정 로직을 TUI 없이 실행하면 서버 운영에서 쓰임새가 크게 넓어집니다. 판정
로직이 이미 순수 함수로 분리되어 있어 비용 대비 효과가 큽니다.

| ID | 항목 | 효과 |
|---|---|---|
| T-016 | `dockviz problems --json`, `dockviz df --json` | 스크립트와 모니터링에서 결과를 그대로 사용 |
| T-017 | 종료 코드 규약 | critical이 있으면 exit 2, warning이면 1. cron이나 CI health check에 바로 연결 |
| T-018 | 기준값 설정 파일 | `~/.config/dockviz/config.yaml`과 프로젝트별 override로 CPU/MEM/storage 기준 조정 |

### 4단계: 정리 기능 다듬기

| ID | 항목 | 효과 |
|---|---|---|
| T-019 | prune 대상 미리보기 | 확인창에서 실제로 지워질 이미지/볼륨 목록과 크기를 보여줌 |
| T-020 | 볼륨 단위 삭제 | 카테고리 전체 prune 대신 볼륨 목록(크기, 라벨, 마지막 사용 컨테이너)에서 하나씩 삭제 |
| T-021 | 로그 rotation 권장 | `max-size`가 없는 컨테이너를 찾아 compose `logging` 설정 예시 제시 |
| T-004 | 원격에서도 동작하는 Container Logs 크기 측정 | T-028(helper 측정 경로)로 대체 예정 |

### 잔존 리소스 탐지

S3 미완료 multipart upload처럼 보이지 않게 남는 Docker 리소스를 찾습니다. 판정 규칙,
환경별 차이, Phase별 완료 기준은 [docs/leftovers-plan.ko.md](docs/leftovers-plan.ko.md)에
있습니다.

| Phase | ID | 항목 | 효과 |
|---|---|---|---|
| 0 | T-023 | Docker context 지원 — 완료 | Docker Desktop, Colima, OrbStack에서 설정 없이 연결 |
| 1 | T-024 | 실측 시나리오 (개발자 검증용, Go) — 완료 | 판정 규칙이 실제 API 필드로 계산되는지 환경별로 확정 |
| 2 | T-025 | 탐지 로직 (읽기 전용, 순수 함수) — 완료 | orphan 컨테이너, 빠진 볼륨, 주인 없는 볼륨, 익명 볼륨, 오래 멈춘 컨테이너 판정 |
| 3 | T-026 | Disk Usage Leftovers 행과 목록 화면 — 완료 | 판정 이유와 확실도까지 한 화면에서 확인 |
| 4 | T-027 | 개별 삭제 + 통합 테스트(T-007) | 확인창, 삭제 직전 재조회, force 금지. CI에서 실제 daemon 검증 |
| 5 | T-028 | VM/원격 측정 경로(helper)와 보고 전용 항목 | `local` 로그 드라이버, Docker Desktop 로그 크기, buildx 캐시, 네트워크 주소 풀, overlay2 차이 |
| 6 | T-029 | 정리 정책 조정 (선택) | 기준 일수 설정, 오탐 반영 |

### 그 밖의 항목

| ID | 항목 | 효과 |
|---|---|---|
| T-001 | 컨테이너 stats 스트리밍 | one-shot 조회의 약 2초 대기 제거. T-009로 부족할 때 검토 |
| T-003 | `--demo`의 정해진 문제 시나리오 | daemon 없이 모든 Problems 신호를 예측 가능한 순서로 재생 (스크린샷/GIF용) |
| T-008 | 크로스 플랫폼 검증 시나리오 | PowerShell 전용 `scenarios/`를 POSIX shell 또는 Go로 이식 |
| T-022 | 목록 검색/정렬 | `/`로 이름 필터, CPU/MEM/크기 기준 정렬 |

### 권장 순서

T-007 → T-006 → T-011·T-012 → T-016·T-017 → T-019 순서를 권장합니다.

- 통합 테스트가 있어야 이후 변경을 안전하게 할 수 있습니다.
- 로그와 추이를 문제 상세에 붙이는 작업이 사용자 체감이 가장 큽니다.
- headless 모드는 기존 판정 로직을 재사용하므로 작은 비용으로 쓰임새를 넓힙니다.

---

## 작업 상세

### T-001: 컨테이너 stats 스트리밍

실행 중 컨테이너마다 `ContainerStats(stream=true)`를 유지해 one-shot 조회의 두 번
측정 대기(컨테이너 12개 병렬 약 2초)를 없앱니다. 컨테이너마다 goroutine과 HTTP
stream이 필요하고, 시작/중지에 따른 수명 관리가 들어갑니다. 갱신이 겹치는 문제는
이미 조회 순번으로 해결했으므로 T-009로 부족할 때 진행합니다.

### T-003: `--demo`의 정해진 문제 시나리오

OOM kill → restart loop → 회복, 메모리 증가 곡선, storage offender 등장을 정해진
타임라인으로 재생합니다. 지금 `DemoClient`는 무작위 event와 현실적인 exit code만
냅니다.

### T-004: 원격에서도 동작하는 Container Logs 크기 측정

> T-028의 helper 측정 경로로 대체할 예정입니다. 아래는 원래 계획입니다.

지금은 `LogPath`를 로컬에서 stat하므로 Docker Desktop과 원격 `DOCKER_HOST`에서는
unavailable입니다. `ContainerLogs`를 스트리밍해 바이트를 세면 어디서나 동작하지만
`stat()`보다 훨씬 비싸므로, 자동 갱신이 아니라 컨테이너별 요청 시에만 실행합니다.
주요 대상인 네이티브 Linux 서버는 이미 동작하므로 우선순위는 낮습니다.

### T-005: 자동 테스트/lint 워크플로 — 완료

`.github/workflows/ci.yml`이 모든 PR과 `main` push에서 gofmt, `go mod tidy` 검사,
3개 OS 대상 `go vet`, ubuntu/macOS/Windows의 `go build`/`go test`(ubuntu는 `-race`)를
실행합니다. `release.yml`도 tag 빌드 전에 테스트를 다시 돌립니다.

### T-006: context를 받는 `DockerClient`

`ListContainers`, `FetchStats`, `DiskUsage`, `Prune*`, `Remove*`에 첫 인자로
`ctx context.Context`를 추가합니다. 갱신 경로에는 지금 내부 timeout이 있지만 호출자가
취소할 수 없고, prune/remove에는 제한이 없습니다. TUI command에서 취소 가능한
context를 넘기고 종료 시 남은 작업을 취소합니다.

### T-007: `Client` 실제 daemon 통합 테스트

build tag나 `DOCKVIZ_INTEGRATION=1`로 켜는 테스트를 만들고, 라벨을 붙인
컨테이너/볼륨/이미지로 `DiskUsage`와 `Prune*` 수치를 확인한 뒤 정리합니다. Docker가
설치된 GitHub `ubuntu-latest` runner에서 실행합니다.
[validation.ko.md](docs/validation.ko.md)의 수동 검증 중 volume prune, 태그 삭제,
Container Logs 항목을 자동화하는 것이 1차 목표입니다.

### T-008: 크로스 플랫폼 검증 시나리오

`scenarios/`의 PowerShell 스크립트를 POSIX shell 또는
`scenarios/stats_parallel_benchmark.go` 옆의 작은 Go 프로그램으로 옮깁니다.

### T-009: stats 동시 조회 수 제한 + 마지막 정상값

semaphore로 동시에 최대 8~16개만 `FetchStats`를 호출하고, 조회에 실패한 컨테이너는
`-` 대신 마지막 정상값을 흐리게 표시합니다(history에는 여전히 넣지 않음).

### T-010: event 기반 즉시 갱신

`die`, `oom`, `kill`, `start` event가 오면 다음 tick을 기다리지 않고 조회를
시작합니다. 진행 중 조회가 있으면 건너뛰는 기존 규칙을 그대로 따릅니다.

### T-011 ~ T-015: 원인까지 보이는 Problems

- **T-011:** `die` event 시각 기준 `ContainerLogs(Since, Until, Tail)`로 전후 로그를
  가져와 문제 상세 아래에 표시합니다. 비용이 있으므로 상세를 열 때만 조회합니다.
- **T-012:** 문제 상세에 해당 컨테이너의 CPU/MEM sparkline과 limit 대비 비율을
  함께 그립니다. 컨테이너가 사라진 뒤에도 보이도록 history를 문제와 함께 보관합니다.
- **T-013:** `health_status` event와 `ContainerInspect().State.Health`를 읽어
  unhealthy를 warning, unhealthy 지속을 critical로 분류합니다.
- **T-014:** Compose context의 `Dependents`를 이용해 문제가 난 service에 의존하는
  service를 "영향 받는 service"로 표시합니다.
- **T-015:** project 요약과 Noisy neighbor 판정을 컨테이너가 아니라 service 단위로
  합산합니다.

### T-016 ~ T-018: headless 모드

- **T-016:** Cobra 서브커맨드 `problems`, `df`를 추가하고 TUI 없이 한 번 조회해 JSON
  또는 표로 출력합니다. CPU/MEM 판정에는 history가 필요하므로 `--sample 10s` 같은
  짧은 수집 시간을 둡니다.
- **T-017:** 출력과 함께 종료 코드로 최고 severity를 알립니다.
- **T-018:** `internal/tui/problems.go`의 상수 기준값을 설정 구조체로 옮기고, 설정
  파일과 플래그로 덮어쓸 수 있게 합니다.

### T-019 ~ T-021: 정리 기능

- **T-019:** prune 확인창에 대상 목록(이름, 크기)을 보여줍니다. Docker API에는
  dry-run이 없으므로 `DiskUsage` 결과에서 대상을 계산합니다. 표시 목록과 실제 대상이
  같아야 한다는 기존 불변식을 그대로 지킵니다.
- **T-020:** Disk Usage에서 Local Volumes 행을 열면 볼륨 목록을 보여주고 하나씩
  삭제합니다. 데이터 손실 경고는 그대로 둡니다.
- **T-021:** inspect의 `HostConfig.LogConfig`에 `max-size`가 없는 컨테이너를 찾아
  Problems에 info로 올리고, compose `logging` 설정 예시를 권장 조치로 보여줍니다.

### T-022, T-023

- **T-022:** 목록에 `/` 검색과 정렬 키를 추가합니다.
- **T-023:** `--context <name>`, `DOCKER_CONTEXT`, `config.json`의 `currentContext`에서 Docker context의 endpoint와 TLS 설정을 사용합니다. 잔존 리소스 탐지의 Phase 0이며 상세는 [docs/leftovers-plan.ko.md](docs/leftovers-plan.ko.md#phase-0-docker-context-지원-t-023)에 있습니다.

### T-002: 이벤트 필터 UI — 취소

지원이 끝난 Events 탭을 대상으로 한 항목입니다. Problems 패널이 이미 event를 장애
관련 신호로 걸러서 보여줍니다.
