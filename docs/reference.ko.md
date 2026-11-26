# dockviz 레퍼런스

화면에 나오는 값이 무엇을 뜻하는지 정리한 문서입니다. 패널별 신호와 기준값,
정리 동작이 실제로 지우는 대상, 컨테이너·이미지 상태를 다룹니다. 대시보드
개요는 [README](../README.ko.md), 계산 방식과 설계 이유는
[TECHNICAL.ko.md](../TECHNICAL.ko.md)를 참고하세요.

---

## 1. Containers 패널

### 컨테이너 상태

| 상태 | 아이콘 | 색상 | 의미 |
|---|---|---|---|
| `running` | ● | 초록 | 컨테이너 프로세스가 실행 중입니다. |
| `exited` | ○ | 빨강 | 프로세스가 종료됐습니다. 파일시스템은 `docker rm` 전까지 보존됩니다. |
| `paused` | ◑ | 노랑 | 모든 프로세스가 `SIGSTOP`으로 정지됐습니다. CPU는 0이 되지만 메모리는 유지됩니다. |
| `restarting` | ↻ | 파랑 | 재시작 정책으로 다시 올라오는 중입니다. `die`와 다음 `start` 사이에 잠깐 보입니다. |
| `dead` | ✗ | 빨강 | 강제 삭제가 실패해 부분적으로만 지워진 상태입니다(예: 장치 사용 중). 수동 정리가 필요합니다. |
| `created` | ○ | 회색 | `docker create`만 실행되고 시작된 적이 없습니다. |
| `removing` | ○ | 회색 | `docker rm`이 진행 중입니다. 끝나면 목록에서 사라집니다. |

### 리소스 열

| 열 | 의미 |
|---|---|
| CPU / MEM | 마지막으로 성공한 stats 샘플. 조회에 실패한 회차는 `-`로 표시하고 history에도 넣지 않습니다. CPU%는 코어 하나를 100%로 보는 Docker CLI 기준이라 여러 코어를 쓰면 100%를 넘을 수 있습니다. |
| CPU95 / MEM95 | 최근 최대 60개 샘플(2초 간격, 약 2분)의 p95 |
| GRAPH | 최근 10개 CPU 샘플의 sparkline. 0~100% 고정 기준이라 다른 컨테이너 부하에 영향받지 않습니다. |
| LIMITS | inspect로 읽은 CPU/MEM hard limit. `-`는 limit 없음, `unknown`은 조회 실패입니다. |

상세 화면(`Enter`)은 current, avg, p95, peak, trend와 Compose service 정보를,
차트(`g`)는 최근 60개 CPU/MEM 샘플을 보여줍니다.

---

## 2. Images 패널

| 표시 | 의미 |
|---|---|
| `이름:태그` | 일반적인 태그 이미지 (예: `nginx:1.25-alpine`) |
| `<none>` | **dangling 이미지.** rebuild나 pull로 태그를 빼앗긴 이전 이미지입니다. 사용되지 않으며 Disk Usage의 Images prune 대상입니다. |

이미지 ID 하나에 태그가 여러 개 붙을 수 있어(예: `nginx:latest`와
`nginx:1.25`) 태그 하나당 한 줄로 표시합니다. 태그를 지우면 그 별칭만 사라지고,
이미지 데이터와 다른 태그는 마지막 태그를 지울 때까지 남습니다. 삭제는
`Force: false`이며 확인창에 다음 중 하나가 표시됩니다.

- 태그가 여러 개: **"Only this tag will be removed. Image remains (N tags total)."**
- 태그가 하나: **"This will remove the image and free disk space."**

---

## 3. Problems 패널

Docker event stream을 내부적으로 수집하지만 정상적인 `create`, `start` 같은
이벤트는 숨기고 조치가 필요한 문제만 표시합니다. CPU/MEM history와, 로드된 경우
Disk Usage 결과도 함께 평가합니다. severity는 색상이 아니라 조치 우선순위이며
critical → warning → info 순으로 정렬합니다.

### 신호

| 신호 | 출처 | 조건 |
|---|---|---|
| **OOM killed** | event | `die` event에 `oomKilled=true` (아래 OOM 참고) |
| **Abnormal exit** | event | `die` event의 exit code가 0이 아님 |
| **Killed** | event | 컨테이너가 kill signal을 받음 |
| **Restart loop** | event | 10분 안에 restart event가 세 번 이상 |
| **CPU saturated / High CPU / Elevated CPU** | CPU history | 최근 5개 샘플 평균이 critical / warning / info 기준을 넘음 |
| **Memory over limit / Memory pressure** | MEM history | `max(현재, p95) / memory limit`이 기준을 넘음 |
| **Memory growth** | MEM history | 샘플 6개 이상, 증가 추세, 처음 대비 `max(50MB, 20%)` 이상 증가. limit의 90% 이상이면 critical |
| **No resource limits** | inspect | 실행 중인데 CPU/MEM hard limit이 모두 없음. CPU p95 ≥ 60% 또는 MEM p95 ≥ 256MB면 warning |
| **Idle memory** | CPU + MEM history | CPU p95 ≤ 5%인데 MEM p95 ≥ 512MB |
| **Noisy neighbor** | project 합산 | 실행 중 2개 이상, project CPU ≥ 50%, 한 컨테이너가 70% 이상 점유 |
| **Storage offenders** | Disk Usage | 큰 로그, build cache, unused image, stopped-container layer, unattached volume, host-storage gap |
| **Daemon disconnected** | event | event stream이 끊김. `r`로 재연결 |

### Severity 기준값

| 신호 | Info | Warning | Critical |
|---|---:|---:|---:|
| CPU 최근 평균 | >=60% | >=80% | >=95% |
| Memory / limit | >=60% | >=80% | >=100% |
| Logs reclaimable | >=100 MB | >=500 MB | >=2 GB |
| Build cache reclaimable | >=500 MB | >=2 GB | - |
| Unused tagged/dangling images | >=100 MB | >=1 GB | - |
| Stopped container writable layers | >=500 MB | >=2 GB | - |
| Unattached volumes | >=100 MB | >=1 GB | - |
| Docker Desktop host-storage gap | >=1 GB | >=10 GB | - |

기준값은 `internal/tui/problems.go`에 있습니다.

### 시간 범위

- 시작할 때 최근 1시간 event를 백필하므로 패널이 처음부터 채워집니다.
- event 기반 판정은 최근 10분을 봅니다.
- 같은 컨테이너에 이후 `start` 또는 `unpause`가 오면 crash/kill 문제는 해제됩니다.
- storage 신호는 Disk Usage 또는 Problems 패널을 한 번 연 뒤(저장공간을 조회한
  뒤)부터 나타납니다.

문제를 선택하고 `Enter`를 누르면 상세 정보, 현재 리소스 상태, Compose service
blast radius(Compose 파일을 읽은 경우), read-only 권장 조치를 볼 수 있습니다.

### 종료 코드

| 코드 | 의미 |
|---|---|
| `0` | 정상 종료 |
| `1` | 일반 애플리케이션 오류 |
| `130` | `SIGINT` (Ctrl+C) |
| `137` | `SIGKILL`. 커널 OOM 또는 `docker kill`. `oomKilled` 속성으로 구분합니다. |
| `143` | `SIGTERM`. `docker stop`의 정상 종료 신호 |
| 기타 | 애플리케이션이 정한 코드. 컨테이너 로그를 확인하세요. |

### OOM

컨테이너 메모리가 `--memory` 한도에 닿으면 커널 OOM killer가 프로세스를 종료합니다.
Docker는 종료 코드 `137`과 `oomKilled=true`를 담은 `die` event를 보내고, dockviz는
이를 critical `OOM killed`로 표시합니다.

```text
critical  OOM killed  api-server  Container was killed by the OOM handler
```

`oomKilled` 없이 `137`만 있으면 `docker kill`일 수도 있으므로 `Abnormal exit`로
분류합니다. 해결은 memory limit 상향, 애플리케이션 메모리 사용량 점검(leak,
캐시 크기), swap limit 설정 중에서 고릅니다.

### 이벤트 액션

dockviz가 event stream에서 읽는 액션입니다. 일반 lifecycle event는 직접 표시하지
않고, 장애와 관련된 `die`, `kill`, `restart`, `start`/`unpause`(해제)만 판정에
씁니다.

| 액션 | 의미 |
|---|---|
| `create` | 컨테이너 객체 생성. 프로세스는 아직 없음 |
| `start` | 프로세스 시작 |
| `restart` | 수동 `docker restart` 또는 재시작 정책으로 재시작 |
| `stop` | `docker stop`이 `SIGTERM`을 보냈고 유예 시간 안에 종료 |
| `kill` | `docker kill`이 signal을 보냄(기본 `SIGKILL`) |
| `die` | 이유와 무관하게 프로세스 종료. 항상 종료 코드가 붙음 |
| `pause` / `unpause` | `SIGSTOP`으로 정지 / 재개 |
| `destroy` | 컨테이너 객체 삭제(`docker rm`) |
| `rename` | 이름 변경 |
| `exec_create` / `exec_start` | 컨테이너 안에서 명령 실행(`docker exec`) |
| `health_status` | `HEALTHCHECK` 결과(healthy / unhealthy / starting) |

---

## 4. Disk Usage 패널

Docker `system/df` API를 기반으로 하며, `docker system df`가 보여주지 않는
저장공간 신호도 함께 표시합니다.

### 카테고리와 정리 동작

| 카테고리 | `d`를 눌렀을 때 |
|---|---|
| Images | dangling 상태의 태그 없는 이미지 삭제 (`docker image prune`과 같고 `-a`가 아님) |
| Containers | 중지된 컨테이너 삭제 |
| Local Volumes | 연결되지 않은 로컬 볼륨 삭제. 이름 있는 볼륨도 포함되므로 애플리케이션 데이터가 지워질 수 있음 |
| Build Cache | 사용되지 않는 build cache layer 삭제 |
| Container Logs | 활성 로그를 그 자리에서 truncate하고 rotated 로그 삭제. 컨테이너는 지우지 않음 |

각 카테고리의 reclaimable 값은 해당 prune이 실제로 회수하는 양과 같도록
계산합니다. 사용되지 않지만 태그가 남은 이미지는 안전한 dangling prune 대상이
아니므로 reclaimable에 합치지 않고 별도 안내로 표시합니다. 이런 이미지는 Images
패널에서 태그를 직접 지웁니다.

앞의 네 카테고리는 Docker API로 처리합니다. Container Logs는 정확한 로그 크기를
주는 portable Docker API가 없어 로컬 파일시스템 작업으로 처리합니다.

### 숫자 읽는 법

- `0B`: Docker가 회수 가능한 용량을 0으로 측정했습니다.
- `N/A`: 크기나 로그 경로를 측정할 수 없는 리소스가 있습니다.
- 행 아래 경고(예: `permission denied on N container(s) — try sudo`): 카테고리
  전체를 측정하지 못한 이유입니다. 조용한 0과 구분하기 위해 표시합니다.
- 패널이 보이는 동안 10초마다 갱신하며 `r`로 즉시 갱신할 수 있습니다.

### Host Storage (Windows Docker Desktop)

Windows Docker Desktop의 WSL2 백엔드에서는 로컬에서 측정 가능한 경우
`docker_data.vhdx`를 읽기 전용 Host Storage 섹션으로 보여줍니다.

- Docker prune은 daemon 내부 객체를 지우지만, VHDX 파일은 Docker Desktop/WSL
  compaction 전까지 Windows에서 크게 남을 수 있습니다.
- 표시되는 gap(VHDX 할당량 중 `system/df`가 설명하지 못하는 부분)은 진단 값이지
  prune 예상 회수량이 아닙니다. Docker Desktop VM 안에서는 이미 비어 재사용
  가능하지만 Windows가 아직 회수하지 않은 공간일 수 있습니다.
- dockviz는 compaction을 실행하지 않습니다. 원격 `DOCKER_HOST`에서는 로컬 VHDX를
  보여주지 않습니다.

실제 측정 사례는 [validation.ko.md](validation.ko.md#3-docker-desktop-vhdx)에
있습니다.

### Container Logs 제약

정확한 로그 크기를 재려면 dockviz와 daemon이 파일시스템을 공유하고, dockviz가
daemon의 로그 디렉터리를 읽을 수 있어야 합니다.

| 환경 | 결과 |
|---|---|
| 네이티브 Linux, rootful `dockerd`, `docker` 그룹 일반 사용자 | 권한 경고 표시 (`try sudo`) |
| 네이티브 Linux, rootful `dockerd`, `sudo dockviz` | 정상 측정, prune 동작 |
| 네이티브 Linux, rootless `dockerd` | `sudo` 없이 정상 측정 |
| `dockerd`를 직접 설치한 WSL2 배포판 | 네이티브 Linux와 같음 |
| Docker Desktop (Windows/macOS), 원격 `DOCKER_HOST` | daemon 파일시스템이 다른 VM/호스트에 있어 경고 없이 0 |

측정 불가를 로그가 없다는 뜻으로 해석하면 안 됩니다. 수동 검증 절차는
[validation.ko.md](validation.ko.md#7-수동-체크리스트-container-logs)에 있습니다.
