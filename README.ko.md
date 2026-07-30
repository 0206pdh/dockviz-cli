# dockviz-cli

> 대용량 실제 daemon 검증 리포트: [docs/core-fixes-validation-report.ko.md](docs/core-fixes-validation-report.ko.md)

**Docker 문제와 디스크 정리를 위한 터미널 대시보드**

[English documentation](README.md) · [Latest releases](https://github.com/0206pdh/dockviz-cli/releases/latest)

## 목적

`dockviz`는 Docker daemon에 직접 연결해 컨테이너 상태와 Docker 저장공간을
한 화면에서 관찰하는 TUI입니다. 핵심 질문은 두 가지입니다.

- 현재 컨테이너에서 실제 장애 신호가 발생하고 있는가?
- Docker 디스크를 무엇이 사용하고 있으며, 무엇을 회수할 수 있는가?

Docker Engine이나 Docker CLI를 대체하는 범용 명령어 래퍼는 아닙니다.

## 패널

| 패널 | 목적 |
|---|---|
| Containers | CPU/MEM, compose/project 요약, p95 요약, limit, 상세 정보, 로그, 추이 차트 |
| Images | 로컬 이미지 태그 조회와 태그/이미지 삭제 |
| Problems | OOM/restart event, high CPU, memory pressure/growth, limit 누락, daemon 연결 문제 |
| Disk Usage | 이미지·컨테이너·볼륨·build cache·로그 분석과 prune |

Networks, 단순 Events 타임라인, `exec`, 컨테이너 start/stop/restart, 이미지
pull 진행률 화면은 현재 제품 범위에서 제외했습니다.

## 스크린샷

| Containers | Container detail |
|---|---|
| ![Containers panel](docs/images/dockviz-containers.svg) | ![Container detail](docs/images/dockviz-container-detail.svg) |

| Problems | Images |
|---|---|
| ![Problems panel](docs/images/dockviz-problems.svg) | ![Images panel](docs/images/dockviz-images.svg) |

| Disk Usage |
|---|
| ![Disk Usage panel](docs/images/dockviz-disk-usage.svg) |

Disk cleanup은 확인창을 거쳐 실행됩니다. Build Cache는 사용 중이 아닌 cache
layer만 정리하고, Local Volumes는 실제 애플리케이션 데이터가 들어 있을 수
있으므로 별도 경고를 표시합니다.

| Build Cache 확인창 | Local Volumes 확인창 |
|---|---|
| ![Build Cache prune confirmation](docs/images/dockviz-confirm-build-cache.svg) | ![Local Volumes prune confirmation](docs/images/dockviz-confirm-volumes.svg) |

## 설치와 업데이트

PyPI 설치:

```bash
python -m pip install dockviz
```

PyPI 업데이트:

```bash
python -m pip install --upgrade dockviz
```

Ubuntu/Debian은 저장소를 등록한 뒤 다음을 실행합니다.

```bash
sudo apt update
sudo apt install dockviz
```

이미 설치된 APT 패키지 업데이트:

```bash
sudo apt update
sudo apt install --only-upgrade dockviz
```

Linux/macOS release binary는 [최신 Release](https://github.com/0206pdh/dockviz-cli/releases/latest)에서
받거나 기존 설치 명령을 다시 실행하면 됩니다. 현재 `dockviz update`
subcommand는 없습니다.

Windows에서는 Release의 `dockviz-windows-amd64.exe` 또는
`dockviz-windows-arm64.exe`를 받으세요.

## 빠른 시작

로컬 daemon 연결:

```bash
dockviz
```

Docker 없이 가상 데이터로 실행:

```bash
dockviz --demo
```

Compose context 자동 탐색:

```bash
dockviz
```

Compose 파일 명시:

```bash
dockviz -f compose.yaml -f compose.override.yaml
```

원격 daemon 연결:

```bash
dockviz --host tcp://192.168.1.100:2375
```

또는 `DOCKER_HOST`를 사용할 수 있습니다. 일반 실행은 시작 전에 Docker
`Ping()`을 호출하며, `--demo`만 실제 daemon 연결을 생략합니다.

## Compose context

Compose 파일이 있으면 `dockviz`는
[`compose-go`](https://github.com/compose-spec/compose-go)로 파일을 읽고,
실제 Docker daemon에서 가져온 실행 중 컨테이너 정보 위에 service context를
겹쳐서 보여줍니다.

자동 탐색 대상:

- `compose.yaml`
- `compose.yml`
- `docker-compose.yaml`
- `docker-compose.yml`

`-f/--compose-file`은 여러 번 사용할 수 있습니다. Compose context는 read-only이며
실제 파일을 수정하지 않습니다.

활용 위치:

- container detail에서 service dependency, dependent, network, configured volume, configured port, Compose 파일 source 표시;
- problem detail에서 문제 컨테이너의 service blast radius 표시;
- Compose 파일이 없어도 live container label과 Docker daemon 정보만으로 계속 동작.

## Problems 패널

Docker event를 내부적으로 수집하지만 정상적인 `create`, `start` 이벤트는
숨기고 운영상 의미 있는 문제만 표시합니다. Containers 패널의 최근 CPU/MEM
history와, 로드된 경우 Disk Usage storage offender도 함께 평가합니다.
severity는 색상이 아니라 조치 우선순위입니다.

- **OOM killed** — OOM handler에 의해 컨테이너가 종료됨;
- **Abnormal exit** — `die` event의 exit code가 0이 아님;
- **Killed** — 컨테이너가 kill signal을 받음;
- **Restart loop** — 10분 안에 세 번 이상 restart됨;
- **CPU saturated / High CPU / Elevated CPU** — 최근 CPU sample 수준에 따라 critical, warning, info로 분류됨;
- **Memory over limit / Memory pressure** — memory p95/current가 configured memory limit을 초과하거나 근접함;
- **Memory growth** — 최근 memory sample이 의미 있게 증가 추세이며 limit에 가까우면 critical로 분류됨;
- **No resource limits** — 실행 중인 컨테이너에 CPU/MEM hard limit이 없음;
- **Idle memory** — CPU는 거의 쓰지 않지만 memory를 크게 잡고 있음;
- **Noisy neighbor** — 한 컨테이너가 project CPU 대부분을 점유함;
- **Storage offenders** — 큰 로그, build cache, unused image, stopped-container layer, unattached volume, Docker Desktop host-storage gap;
- **Daemon disconnected** — Docker event stream이 끊김.

현재 threshold:

| Signal | Info | Warning | Critical |
|---|---:|---:|---:|
| CPU recent average | >=60% | >=80% | >=95% |
| Memory / limit | >=60% | >=80% | >=100% |
| Logs reclaimable | >=100 MB | >=500 MB | >=2 GB |
| Build cache reclaimable | >=500 MB | >=2 GB | - |
| Unused tagged/dangling images | >=100 MB | >=1 GB | - |
| Stopped container writable layers | >=500 MB | >=2 GB | - |
| Unattached volumes | >=100 MB | >=1 GB | - |
| Docker Desktop host-storage gap | >=1 GB | >=10 GB | - |

이후 `start` 또는 `unpause` event가 오면 crash/kill 문제는 해결된 것으로
처리합니다. 최초 event 조회는 최근 1시간, 문제 판정은 최근 10분 기준입니다.
Problems 패널에서 `[Enter]`를 누르면 문제 상세 정보, 현재 resource context,
read-only recommendation을 확인할 수 있습니다.

## Disk Usage 패널

Docker `system/df` API를 기반으로 하며, `docker system df`에 없는
저장공간 신호도 함께 보여줍니다.

| 카테고리 | 동작 |
|---|---|
| Images | dangling 상태의 태그 없는 이미지 삭제 |
| Containers | 중지된 컨테이너 삭제 |
| Local Volumes | 연결되지 않은 로컬 볼륨 삭제 — 데이터 손실 주의 |
| Build Cache | 사용되지 않는 build cache layer 삭제 |
| Container Logs | 활성 로그 truncate 및 rotated 로그 삭제 |

Windows Docker Desktop의 WSL2 백엔드에서는 로컬에서 측정 가능한 경우
`docker_data.vhdx`를 읽기 전용 Host Storage 섹션으로 표시합니다. 이 값은
prune 표와 의도적으로 분리되어 있습니다. Docker prune은 daemon 내부 객체를
삭제하지만, VHDX 파일은 Docker Desktop/WSL compaction 전까지 Windows에서
계속 크게 보일 수 있습니다.

가능한 경우 dockviz는 VHDX 할당량 중 Docker `system/df`가 설명하지 못하는
차이도 표시합니다. 이 값은 진단용 gap이며 prune 예상 회수량이 아닙니다.

행을 선택하고 `d`를 누르면 확인창이 표시됩니다. 로그 정리는 정확한 파일
크기를 제공하는 portable Docker API가 없기 때문에 로컬 파일시스템 작업으로
수행됩니다.

큰 Host Storage VHDX 값을 Docker reclaimable 용량으로 해석하면 안 됩니다.
이 값은 host-side 가상 디스크 할당량이며, Docker Desktop VM 내부에서는 이미
비어 있어 재사용 가능한 공간이지만 Windows가 아직 회수하지 않은 공간일 수
있습니다.

Container Logs의 정확한 측정에는 daemon과 dockviz의 파일시스템 공유 및
로그 디렉터리 접근 권한이 필요합니다. Docker Desktop이나 원격
`DOCKER_HOST`에서는 daemon 파일시스템이 다른 VM/호스트에 있어 unavailable
또는 0으로 보일 수 있습니다. 측정 불가를 로그가 없다는 뜻으로 해석하면
안 됩니다.

## 키보드 단축키

| 키 | 동작 |
|---|---|
| `q` / `Ctrl+C` | 종료 |
| `Tab` | Containers, Images, Problems, Disk Usage 전환 |
| `↑` / `k`, `↓` / `j` | 목록 이동 |
| `Enter` | 컨테이너 상세 정보 |
| `Esc` | 뒤로 가기 / overlay 닫기 |
| `d` | 컨테이너/이미지 삭제 또는 Disk Usage prune |
| `l` | 실시간 로그 |
| `r` | 새로고침 및 event stream 재연결 |
| `g` | CPU/MEM 추이 차트 |

## Docker 연결

실제 client는 `DOCKER_HOST`와 Docker의 표준 환경 설정을 사용하며,
`--host`가 지정되면 `DOCKER_HOST`보다 우선합니다. 현재 사용자가 Docker
socket 또는 endpoint에 접근할 수 있어야 합니다. 인증 없는 원격 Docker TCP
socket은 원격 root 권한과 같으므로 TLS와 네트워크 접근 제어를 사용하세요.

## 소스 빌드와 개발

```bash
git clone https://github.com/0206pdh/dockviz-cli.git
cd dockviz-cli
go build -o dockviz .
```

소스 업데이트는 `git pull` 후 다시 `go build`하면 됩니다.

```bash
go test ./...
go vet ./...
go build ./...
```

주요 구현 파일은 다음과 같습니다.

- `internal/docker/client.go` — daemon 연결;
- `internal/docker/containers.go` — 컨테이너 목록과 stats;
- `internal/docker/images.go` — 이미지 조회/삭제;
- `internal/docker/diskusage.go` — 저장공간 집계와 정리;
- `internal/docker/events.go` — lifecycle event stream;
- `internal/docker/logs.go` — 실시간 로그;
- `internal/tui/problems.go` — event를 문제로 분류;
- `internal/tui/view.go`, `internal/tui/update.go` — TUI 화면과 상태 전환.

실제 SDK client와 demo 구현은 같은 `DockerClient` interface를 구현하므로
daemon 없이도 TUI와 판정 로직을 테스트할 수 있습니다.

## Release

Release 자동화는 `.github/workflows/release.yml`에 있습니다. `v1.2.3` 같은
version tag를 push했을 때만 release가 생성됩니다. 일반 `main` push는 release나
patch tag를 자동 생성하지 않습니다. version 주입 빌드는
`go build -ldflags="-X main.version=v1.2.3" -o dockviz .`로 합니다.
`ldflags` 없는 로컬 빌드의 버전은 `dev`입니다.

## 정량 성능 시나리오

실제 Docker daemon 연결과 Containers, Problems, Disk Usage 패널을 숫자로
검증하려면 [`scenarios/run-dockviz-performance.ps1`](scenarios/run-dockviz-performance.ps1)을
실행한다. CPU, 메모리, 로그 폭증, 재시작 루프와 max-safe 저장소 부하를 만들고
CSV/JSON 결과를 `artifacts`에 저장한 뒤 테스트 컨테이너와 라벨된 volume을 자동으로 정리한다.
노트북의 여유 공간을 크게 사용하는 검증은 `-UseMaxSafeStorage`를 사용할 수
있으며, 기본 12 GiB를 남기고 나머지를 payload로 사용한다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scenarios\run-dockviz-performance.ps1 `
  -RunLabel max-safe `
  -UseMaxSafeStorage `
  -StorageReserveGB 12 `
  -DurationSeconds 20 `
  -StorageReadyTimeoutSeconds 1800
```

자세한 설명은
[`docs/performance-scenarios.ko.md`](docs/performance-scenarios.ko.md)를 참고한다.
실제 daemon에서 실행한 검증 결과는
[`docs/performance-results.ko.md`](docs/performance-results.ko.md)에 정리되어 있다.
CPU/MEM health 시나리오 smoke 결과는
[`docs/resource-health-smoke-results.ko.md`](docs/resource-health-smoke-results.ko.md)에 정리되어 있다.
unused tagged image, dangling image, unused volume, Docker Desktop VHDX
동작을 실제 daemon에서 검증한 상세 리포트는
[`docs/reclaim-validation-report.ko.md`](docs/reclaim-validation-report.ko.md)에 정리되어 있다.

CPU/MEM 문제 탐지 전용 시나리오는
[`scenarios/run-dockviz-resource-health.ps1`](scenarios/run-dockviz-resource-health.ps1)에 있다.

CPU/MEM 관리 기능의 phase 로드맵은
[`docs/resource-management-roadmap.ko.md`](docs/resource-management-roadmap.ko.md)에 정리되어 있다.

## License

MIT © 2026 [0206pdh](https://github.com/0206pdh)
