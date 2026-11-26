# dockviz 검증 기록

단위 테스트로는 확인할 수 없는 동작(실제 daemon의 prune 결과, Docker Desktop VHDX,
stats 조회 시간, 로그 파일 권한)을 실제 Docker 환경에서 확인한 기록과 재현
방법입니다.

- 기록된 실행은 모두 Windows Docker Desktop WSL2, Docker client/server 28.5.1
  환경입니다.
- 원본 산출물(`artifacts/`)은 `.gitignore` 대상이라 커밋하지 않고, 재현 명령과
  핵심 수치만 이 문서에 남깁니다.
- 시나리오는 전역 prune을 호출하지 않고, 라벨(`dockviz.scenario`,
  `dockviz.validation`)이 붙은 테스트 리소스만 정리합니다.

---

## 0. 요약

| 확인 항목 | 결과 | 실행 |
|---|---|---|
| named volume prune | `all=true` 없이 `0B`, 있으면 `8.59GB` 회수 | core-fixes, 2026-07-30 |
| Docker Desktop VHDX | Docker 객체를 다 지워도 VHDX는 줄지 않음. compaction 후 `19.46GB → 5.47GB` | reclaim / max-safe, 2026-07-29 |
| 태그 단위 이미지 삭제 | 813MB multi-tag 이미지에서 선택한 태그만 제거 | core-fixes |
| tagged unused / dangling 구분 | 2.15GB tagged unused는 dangling prune 대상 아님, 1.62GB dangling은 대상 | reclaim |
| build cache 잔여 | 이미지 삭제 후에도 806.1MB 남음 → Build Cache prune으로 회수 | core-fixes |
| stats 병렬화 | 컨테이너 12개 `19.730s → 2.058s` (9.585배) | core-fixes |
| 저장소 압박 + 문제 동시 관측 | 17.18GB volume, CPU 94%, restart 20회를 한 실행에서 확인 | max-safe, 2026-07-29 |
| CPU/MEM 문제 탐지 | CPU hog, memory pressure/growth, no-limit 컨테이너 재현 | resource-health, 2026-07-30 |

---

## 1. 시나리오와 실행 방법

| 스크립트 | 목적 |
|---|---|
| `scenarios/run-core-fixes-validation.ps1` | volume prune, VHDX, 태그 삭제, stats 병렬화를 대용량 fixture로 검증 |
| `scenarios/run-dockviz-performance.ps1` | CPU, 메모리, 로그, restart loop, 대용량 volume 부하를 동시에 생성 |
| `scenarios/run-dockviz-resource-health.ps1` | Problems 패널의 CPU/MEM 신호를 재현 |
| `scenarios/stats_parallel_benchmark.go` | dockviz와 같은 Go SDK 경로로 순차/병렬 stats 비교 |
| `scenarios/compare-performance-results.ps1` | 두 실행 결과 비교 |

모든 스크립트는 Docker daemon이 실행 중인 저장소 루트에서 실행합니다. 실행하는
동안 다른 터미널에서 `go run .`을 띄우면 Containers, Problems, Disk Usage 패널을
함께 관찰할 수 있습니다. 현재는 PowerShell 전용입니다(TODOS T-008).

### core-fixes

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scenarios\run-core-fixes-validation.ps1 `
  -OutputDirectory .\artifacts\core-fixes-validation `
  -VolumeSizeGB 4 `
  -ImageLayerMB 768 `
  -StatsContainerCount 12 `
  -StatsRuns 5
```

### max-safe 성능 시나리오

```powershell
powershell -ExecutionPolicy Bypass -File .\scenarios\run-dockviz-performance.ps1 `
  -RunLabel max-safe `
  -UseMaxSafeStorage `
  -StorageReserveGB 12 `
  -DurationSeconds 20 `
  -SampleIntervalSeconds 2 `
  -StorageReadyTimeoutSeconds 1800 `
  -OutputDirectory .\artifacts
```

`-UseMaxSafeStorage`는 작업 폴더 드라이브의 여유 공간에서 예약 공간(기본 12GiB)을
뺀 정수 GiB를 payload로 씁니다. Windows와 Docker Desktop이 계속 동작할 공간을
남기기 위해 100%까지 채우지 않습니다. payload는 `/dev/urandom`으로 기록해 실제
블록을 할당합니다.

만드는 부하:

| 컨테이너 | 역할 |
|---|---|
| CPU | CPU loop로 지속 부하 |
| Memory | 64MiB 파일을 기록하고 유지 |
| Logs | 20,000줄 출력 |
| Restart | 2초마다 exit 42, `on-failure:20` |
| Storage | max-safe random payload를 volume에 기록 |

결과 파일(`artifacts/`):

| 파일 | 내용 |
|---|---|
| `*-summary.json` | 컨테이너별 CPU 평균/p95, 메모리 p95, restart 횟수, max-safe 계산값 |
| `*-samples.csv` | 샘플별 원시 측정값 |
| `*-before-system-df.txt` | 테스트 전 Docker 디스크 사용량 |
| `*-workload-system-df.txt` | volume이 active인 상태 |
| `*-reclaimable-system-df.txt` | storage 컨테이너 제거 후 미사용 volume 상태 |
| `*-after-cleanup-system-df.txt` | 정리 후 상태 |

애플리케이션 자체의 Docker-level CPU/메모리를 비교하려면 같은 고정 workload를
`-TargetImage my-app:baseline -TargetCommand "./run-fixed-workload.sh"`와
`my-app:improved`에 각각 실행합니다. HTTP 성능(RPS, 오류율, p50/p95 latency)은
별도 workload generator로 수집해야 합니다.

### resource-health

```powershell
powershell -ExecutionPolicy Bypass -File .\scenarios\run-dockviz-resource-health.ps1 `
  -RunLabel resource-health `
  -DurationSeconds 30 `
  -SampleIntervalSeconds 2 `
  -OutputDirectory .\artifacts
```

compose-style label을 가진 CPU hog, memory pressure, memory growth, limit 없는
컨테이너를 만듭니다.

---

## 2. Local Volumes prune

**문제:** Disk Usage가 Local Volumes reclaimable을 보여줘도 실제 prune이 `0B`로
끝나는 경우가 있었습니다. Docker API 1.42부터 필터 없는 volume prune은 익명
volume만 대상으로 하므로, 컨테이너가 지워진 뒤 남은 named volume이 지워지지
않습니다. 버그 기록은 [troubleshooting.ko.md](troubleshooting.ko.md)의
"Local Volumes 프룬이 표시된 것보다 적게 회수됨" 항목입니다.

**fixture:** 같은 label의 named volume 2개에 각각 4GiB를 기록했습니다(Docker df
기준 각 4.295GB, 합계 8.59GB). 실행 ID `dockviz-core-20260730142414`.

| prune 방식 | 결과 |
|---|---|
| label filter만, `--all` 없음 | `Total reclaimed space: 0B`, volume 그대로 남음 |
| 같은 filter + `--all` | 두 volume 삭제, `Total reclaimed space: 8.59GB` |

**결론:** Local Volumes prune은 `all=true`를 명시해야 화면의 reclaimable과 실제
삭제 대상이 일치합니다. 현재 코드는 `VolumesPrune(all=true)`를 사용합니다.

---

## 3. Docker Desktop VHDX

**문제:** Docker 객체를 지워도 Windows 디스크 여유 공간이 바로 돌아오지 않습니다.
daemon 안에서는 지워졌지만, Docker Desktop WSL2의 `docker_data.vhdx`는 compaction
전까지 Windows에 크게 할당된 채 남기 때문입니다.

reclaim 실행(`dockviz-reclaim-complete-20260729074256`)의 기록:

| 시점 | C: free | `docker_data.vhdx` |
|---|---:|---:|
| 시작 | 25.78GB | 7.97GB |
| tagged unused image 생성 후 | 25.06GB | 8.69GB |
| dangling image 생성 후 | 24.37GB | 9.38GB |
| unused volume 생성 후 | 18.84GB | 9.38GB |
| Docker 객체 삭제 직후 | 18.84GB | 9.38GB |
| 이후 재확인 | 23.75GB | 9.38GB |

core-fixes 실행에서도 8.59GB volume을 만들고 지운 뒤 VHDX는 9.375GB, host 여유
공간은 13.512GB로 그대로였습니다.

max-safe 실행 뒤에는 Ubuntu WSL은 건드리지 않고 Docker Desktop만 멈춘 상태에서
VHDX를 compact했습니다.

```text
docker_data.vhdx    19.46GB -> 5.47GB
C: free space       13.93GB -> 28.03GB
```

**결론:** `docker system df`가 0B여도 VHDX는 자동으로 줄지 않습니다. 이것은 prune
실패가 아니라 host 쪽 가상 디스크 할당 문제입니다. 그래서 dockviz는 Docker
reclaimable과 Host Storage를 분리해 보여주고, gap에는 삭제 버튼 대신 compaction
안내만 붙입니다. 다른 WSL 배포판까지 멈추는 `wsl --shutdown`은 쓰지 않습니다.

---

## 4. 이미지 정리

### tagged unused와 dangling

reclaim 실행에서 컨테이너가 쓰지 않는 이미지 두 종류를 만들었습니다.

| 유형 | 크기 | dangling prune 대상 | dockviz에서 지우는 곳 |
|---|---:|---|---|
| tagged unused (`...-unused:latest`, containers=0) | 2.15GB | 아님. 태그가 살아 있음 | Images 패널에서 태그 삭제 |
| dangling (`<none>:<none>`) | 1.62GB (unique 1.613GB) | 맞음 | Disk Usage Images prune |
| unused volume (links=0) | 2.147GB | - | Disk Usage Local Volumes prune |

합계 약 5.91GB를 찾았고 각각의 경로로 모두 지웠습니다. tagged unused 이미지는
Disk Usage 숫자에 합치지 않고 별도 안내로 보여주는 것이 맞다는 근거입니다.

### multi-tag 이미지의 태그 단위 삭제

같은 이미지 ID에 태그가 여러 개일 때 `Force: true`로 지우면 태그 하나만 지우려
해도 이미지와 layer까지 사라질 수 있습니다. 캐시를 피하려고 `--no-cache`로
768MiB payload 이미지(813MB)를 새로 빌드하고 `:keep`, `:remove` 두 태그를
붙였습니다.

```text
Untagged: dockviz-core-20260730142414-image:remove

keptTagExistsAfterRemove:    true
removedTagExistsAfterRemove: false
```

선택한 태그만 사라지고 다른 태그와 layer는 유지됐습니다. 현재 이미지 삭제는
`Force: false`입니다.

---

## 5. Build cache

이미지 태그와 layer를 지운 뒤에도 테스트 빌드가 만든 BuildKit cache
`806.1MB`가 남았고, `docker builder prune --force`로 `806.1MB`를 회수했습니다.
이미지를 지웠는데도 공간이 남으면 Images가 아니라 Build Cache prune이 맞는
조치라는 근거입니다. Build Cache prune은 rebuild 시간을 늘릴 수 있어 확인창을
거칩니다.

---

## 6. 리소스와 문제 탐지

### 6.1 stats 병렬화

dockviz와 같은 Go SDK 경로(`internal/docker.FetchStats`)를 컨테이너 12개에 대해
순차와 goroutine 병렬로 5회씩 호출했습니다.

| run | 순차 | 병렬 | 배수 |
|---:|---:|---:|---:|
| 1 | 19.494s | 2.091s | 9.325x |
| 2 | 20.092s | 2.436s | 8.248x |
| 3 | 19.580s | 1.936s | 10.113x |
| 4 | 21.141s | 1.879s | 11.248x |
| 5 | 18.343s | 1.949s | 9.409x |
| **평균** | **19.730s** | **2.058s** | **9.585x** |

병렬화는 daemon 응답 자체를 빠르게 하지 않고 여러 컨테이너 조회의 wall-clock
시간을 줄입니다. 병렬 평균도 2초 tick보다 길 수 있어, TUI는 조회가 진행 중이면
다음 tick을 건너뛰고 늦게 도착한 옛 결과는 버립니다
([TECHNICAL.ko.md §4](../TECHNICAL.ko.md#4-데이터-갱신)).

### 6.2 저장소 압박과 문제 동시 관측 (max-safe)

실행 ID `dockviz-perf-20260729061036`. 여유 공간 28GB에서 12GB를 남기고 16GiB
payload를 썼습니다(전체 약 8분 24초, workload 관찰 20초).

| 시점 | Local Volumes | reclaimable |
|---|---:|---:|
| workload 실행 중 | 17.18GB, active 1 | 0B |
| storage 컨테이너만 제거 | 17.18GB, active 0 | 17.18GB (100%) |

| runtime 지표 | 값 |
|---|---:|
| CPU 컨테이너 평균 / p95 | 94.10% / 99.76% |
| Memory 컨테이너 p95 | 0.98MB |
| 로그 출력 | 0.50MB |
| Restart 컨테이너 재시작 | 20회 |

컨테이너 목록만 보면 이미 지워졌지만, Disk Usage에서는 17.18GB volume이 100%
reclaimable로 남아 있음을 바로 확인할 수 있었습니다. 같은 실행에서 CPU 과부하와
restart loop도 Problems에 함께 나타났습니다.

### 6.3 CPU/MEM health smoke

2026-07-30, project label `codex-smoke-dockviz-health-20260730042422`, 30초.

| 컨테이너 | 샘플 | 최대 CPU | 최대 메모리 |
|---|---:|---:|---:|
| `cpu-hog` | 7 | 91.92% | 0.445MB |
| `mem-pressure` | 7 | 0% | 56.56MB |
| `mem-growth` | 7 | 3.75% | 80.64MB |
| `no-limit` | 7 | 0% | 0.434MB |

확인할 지점:

- Containers 패널 상단에 project resource summary가 보입니다.
- Problems에 `High CPU`/`CPU saturated`, `Memory pressure`/`Memory over limit`,
  `Memory growth`, `No resource limits`가 뜹니다.
- Problems에서 `Enter`로 상세와 권장 조치가 보입니다.

---

## 7. 수동 체크리스트: Container Logs

Container Logs는 `LogPath`를 로컬에서 stat/truncate하므로, 동작 여부가 코드가
아니라 환경(파일시스템 공유, `/var/lib/docker/containers` 권한)에 달려 있습니다.
`internal/docker/diskusage.go`를 바꾸는 릴리스 전에 네이티브 Linux에서 실행합니다.
환경별 예상 결과는 [reference.ko.md](reference.ko.md#container-logs-제약)의 표를
따릅니다.

1. **결정적인 크기의 로그 만들기**

   ```bash
   docker run -d --name logtest alpine sh -c '
     i=0
     while [ $i -lt 5000 ]; do
       echo "line $i $(head -c 100 </dev/zero | tr "\0" x)"
       i=$((i+1))
     done
     sleep 3600'
   LOGPATH=$(docker inspect --format='{{.LogPath}}' logtest)
   stat -c%s "$LOGPATH"
   ```

2. **패널과 비교:** `sudo dockviz` → Disk Usage → Container Logs의 SIZE가
   `실측 바이트 / 1024 / 1024`와 반올림 단위 안에서 같아야 합니다. 프룬 전
   `ReclaimMB == SizeMB`이어야 합니다.
3. **프룬:** Container Logs에서 `d` → 확인. `stat -c%s "$LOGPATH"`가 `0`이어야
   합니다.
4. **컨테이너 생존:** 프룬 전부터 `docker logs -f logtest`를 띄워 두고 프룬 뒤
   `docker exec logtest sh -c 'echo post-truncate-line'`. 컨테이너가 `running`이고
   새 줄이 계속 찍혀야 합니다(daemon의 fd가 truncate를 견딤).
5. **rotated 파일**

   ```bash
   docker run -d --name logtest-rotate --log-opt max-size=1k --log-opt max-file=3 \
     alpine sh -c 'i=0; while [ $i -lt 5000 ]; do echo "line $i padding padding padding"; i=$((i+1)); done; sleep 3600'
   ls "$(docker inspect --format='{{.LogPath}}' logtest-rotate)"*
   ```

   `.log.1`, `.log.2`가 SIZE에 포함되고(`du -cb "$LOGPATH"*`와 비교), 프룬 뒤에는
   완전히 사라지며 활성 `.log`만 0바이트로 남아야 합니다.
6. **권한 거부:** root 소유 `/var/lib/docker`에서 `docker` 그룹 일반 사용자로
   1~2단계를 반복합니다. 값은 0이지만 `permission denied on N container(s) — try
   sudo` 경고가 보여야 합니다.
7. **미지원 환경:** Docker Desktop이나 원격 `DOCKER_HOST`에서 경고 없이 0이 나오고
   패닉이 없어야 합니다.

정리: `docker rm -f logtest logtest-rotate`

---

## 8. 수동 체크리스트: CPU/MEM 차트

차트(`g`)의 임계선, 막대 높낮이, 색 전환을 보려면 변동이 있는 부하가 필요합니다.
항상 100%(`progrium/stress`)이거나 항상 낮은(`sleep infinity`) 컨테이너로는 변화가
보이지 않습니다.

```bash
# 4초 부하 / 6초 휴식 반복
docker run -d --name cpu-pulse alpine sh -c "
while true; do yes > /dev/null & PID=\$!; sleep 4; kill \$PID; sleep 6; done"

# 강약을 번갈아 가며 부하
docker run -d --name cpu-wave alpine sh -c "
while true; do
  dd if=/dev/urandom of=/dev/null bs=1M count=200 2>/dev/null; sleep 8
  dd if=/dev/urandom of=/dev/null bs=1M count=50 2>/dev/null; sleep 8
done"

# 단계적으로 올라갔다 내려오는 패턴
docker run -d --name cpu-sin alpine sh -c "
i=0
while true; do
  i=\$((i+1)); mod=\$((i % 10))
  if [ \$mod -lt 5 ]; then yes > /dev/null & PID=\$!; sleep \$((mod + 1)); kill \$PID 2>/dev/null
  else sleep 2; fi
done"
```

두세 개를 함께 띄우고 차트에서 컨테이너를 바꿔 가며 비교합니다. 정리:
`docker rm -f cpu-pulse cpu-wave cpu-sin`

---

## 9. 해석과 한계

검증으로 확인한 dockviz의 역할은 Docker 명령을 대신 실행하는 것이 아니라, 흩어진
storage/resource/problem 신호를 한 화면에 모으고, 실제로 회수 가능한 대상과 진단만
가능한 대상을 나누며, 위험한 조치에 확인창을 거는 것입니다.

의도적으로 자동 삭제를 제공하지 않는 영역:

- **tagged unused image:** 태그의 의미를 사용자가 확인한 뒤 Images 패널에서 삭제
- **Docker Desktop VHDX:** prune이 아니라 compaction 대상. Docker Desktop/WSL 상태와
  관리자 권한에 따라 위험하므로 read-only 진단만 제공
- **Build Cache:** 전역 cache 삭제는 개발 workflow에 영향을 주므로 확인창 필요
- **Local Volumes:** 실제 데이터 손실 가능성이 있어 가장 강한 경고
- **원격 host의 VHDX:** 로컬 Windows 값을 보여주면 틀린 정보가 되므로 unavailable

테스트 범위의 한계:

- 기록된 실행은 모두 Windows Docker Desktop입니다. 네이티브 Linux 수치는 §7
  체크리스트로 따로 확인해야 합니다.
- 시나리오는 HTTP latency나 RPS를 재는 애플리케이션 벤치마크가 아닙니다.
- 이 절차들은 수동입니다. 실제 daemon을 쓰는 자동 테스트는 TODOS T-007입니다.
