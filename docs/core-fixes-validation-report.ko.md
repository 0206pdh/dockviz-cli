# dockviz 핵심 개선 대용량 검증 리포트

측정 시각: 2026-07-30 23:28 KST  
실행 ID: `dockviz-core-20260730142414`  
Docker Server: `28.5.1`  
환경: Windows Docker Desktop WSL2  
원본 증거 위치: `artifacts/core-fixes-validation/`  

`artifacts/`는 `.gitignore` 대상이라 저장소에는 원본 산출물을 커밋하지 않는다. 대신 이 문서에 재현 명령, 핵심 수치, 해석을 남긴다.

## 1. 검증 목적

이번 검증은 “dockviz가 단순히 Docker 명령어를 감싸는 수준인지”가 아니라, 실제 daemon에서 다음 네 가지 개선이 쓸모 있는지 확인하기 위한 것이다.

1. named local volume이 reclaimable로 보이는데도 prune되지 않던 문제 해결
2. Docker 객체 삭제와 Windows Docker Desktop VHDX 축소가 별개라는 점을 UI에서 분리 진단
3. 이미지 태그 하나 삭제 시 같은 이미지 ID의 다른 태그까지 지워지는 위험 제거
4. 컨테이너 수가 늘어날 때 stats refresh가 느려지던 문제를 Go goroutine 병렬화로 완화

사용한 대용량 fixture는 다음과 같다.

| 항목 | 크기/개수 | 목적 |
|---|---:|---|
| named volume A | 4GiB payload, Docker df 기준 4.295GB | `docker volume prune` 기본 동작 검증 |
| named volume B | 4GiB payload, Docker df 기준 4.295GB | `docker volume prune --all` 동작 검증 |
| 테스트 이미지 | 768MiB payload, image size 813MB | tag-safe 삭제 검증 |
| stats 컨테이너 | 12개 | 순차 stats 조회와 병렬 stats 조회 비교 |

실행 명령:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File .\scenarios\run-core-fixes-validation.ps1 `
  -OutputDirectory .\artifacts\core-fixes-validation `
  -VolumeSizeGB 4 `
  -ImageLayerMB 768 `
  -StatsContainerCount 12 `
  -StatsRuns 5
```

## 2. Local Volumes prune 검증

### 문제

Disk Usage 패널이 Local Volumes reclaimable 용량을 보여줘도 실제 prune 결과가 `0B`로 끝나는 경우가 있었다. 핵심 원인은 Docker API 1.42 이후의 volume prune 기본 동작이다. `all=true`가 없으면 daemon이 anonymous volume 위주로 제한해서, 컨테이너가 삭제된 뒤 남은 named volume이 삭제되지 않을 수 있다.

### 실제 fixture

스크립트가 같은 label을 가진 named volume 2개를 만들고 각각 4GiB 파일을 기록했다.

```text
dockviz-core-20260730142414-volume-all      4.295GB
dockviz-core-20260730142414-volume-no-all   4.295GB
```

합계는 Docker `system df -v` 기준 8.59GB다.

### 결과

`--all` 없이 label filter만 걸고 prune:

```text
Total reclaimed space: 0B
```

이후 `volume-no-all`은 그대로 존재했다.

`--all`을 명시하고 같은 label filter로 prune:

```text
Deleted Volumes:
dockviz-core-20260730142414-volume-no-all
dockviz-core-20260730142414-volume-all

Total reclaimed space: 8.59GB
```

결론: dockviz의 Local Volumes prune은 `all=true`를 명시해야 UI가 보여주는 reclaimable named volume과 실제 삭제 대상이 일치한다. 이 검증에서는 8.59GB가 실제 회수됐다.

## 3. Docker Desktop VHDX / Host Storage 검증

### 문제

Docker 객체를 삭제해도 Windows C: 드라이브 여유 공간이 바로 돌아오지 않을 수 있다. Docker daemon 안에서는 volume/image/cache가 지워졌지만, Docker Desktop WSL2의 `docker_data.vhdx` 파일은 compact 전까지 Windows host에 계속 크게 할당될 수 있기 때문이다.

### 이번 실행의 기록

```text
hostBefore.freeGB:        13.520GB
hostAfterWorkload.freeGB: 13.512GB
docker_data.vhdx:          9.375GB
after cleanup host free:  13.512GB
```

이번 실행에서는 VHDX가 이미 앞선 대용량 검증으로 9.375GB까지 커져 있었기 때문에, 8.59GB volume fixture가 추가로 host 파일을 크게 확장하지는 않았다. 중요한 관찰은 Docker 객체 cleanup 뒤에도 VHDX 파일 크기와 Windows host free space가 자동으로 회복되지 않았다는 점이다.

dockviz의 Disk Usage 패널이 Docker reclaimable과 Host Storage VHDX를 분리해서 보여줘야 하는 이유가 여기에 있다.

- Docker reclaimable: daemon 내부 객체 삭제로 회수 가능한 용량
- Host Storage VHDX: Windows가 아직 들고 있는 VM disk allocation
- VHDX gap: prune 실패가 아니라 compact/운영 안내가 필요한 진단 값

## 4. 이미지 태그 단위 삭제 검증

### 문제

같은 image ID에 여러 태그가 붙어 있을 때 `Force: true`로 image remove를 호출하면, 사용자가 선택한 태그 하나만 지우려던 상황에서도 실제 이미지/layer까지 제거될 수 있다.

### 실제 fixture

캐시 재사용을 막기 위해 `--no-cache`와 scenario build arg를 넣고 768MiB payload 이미지를 새로 빌드했다.

빌드 로그의 핵심 기록:

```text
RUN dd if=/dev/zero of=/dockviz-image-payload.bin bs=1M count=768
805306368 bytes (768.0MB) copied
```

빌드된 이미지:

```text
dockviz-core-20260730142414-image:keep   813MB
dockviz-core-20260730142414-image:remove 813MB
```

### 결과

선택한 태그만 삭제:

```text
Untagged: dockviz-core-20260730142414-image:remove
```

검증 결과:

```text
keptTagExistsAfterRemove:    true
removedTagExistsAfterRemove: false
```

즉 `:remove` 태그만 사라지고 `:keep` 태그와 실제 이미지 layer는 유지됐다. dockviz의 Images 패널은 태그 하나당 한 줄로 보여주고, 멀티태그 상황에서는 사용자가 어떤 태그를 지우는지 명확히 보여줘야 한다.

## 5. stats refresh 병렬화 검증

### 문제

Docker stats 단발 조회는 컨테이너별로 시간이 걸린다. 이를 순차로 처리하면 컨테이너 수에 거의 비례해서 TUI refresh가 느려진다.

이번 검증에서는 PowerShell job이 아니라 dockviz와 같은 Go SDK 경로를 사용했다. `internal/docker.FetchStats`를 순차 호출한 경우와 Go goroutine으로 병렬 호출한 경우를 같은 12개 컨테이너에 대해 5회 반복했다.

원본 측정 파일:

```text
artifacts/core-fixes-validation/04-parallel-stats/stats-runs.csv
artifacts/core-fixes-validation/04-parallel-stats/stats-summary.json
```

### 결과

| run | sequential | parallel | speedup |
|---:|---:|---:|---:|
| 1 | 19.494s | 2.091s | 9.325x |
| 2 | 20.092s | 2.436s | 8.248x |
| 3 | 19.580s | 1.936s | 10.113x |
| 4 | 21.141s | 1.879s | 11.248x |
| 5 | 18.343s | 1.949s | 9.409x |

평균:

```text
sequentialAvgSeconds: 19.730s
parallelAvgSeconds:    2.058s
speedup:               9.585x
```

결론: 이 개선은 “보여주는 정보가 늘었다” 수준이 아니라, 컨테이너가 12개만 되어도 refresh latency를 약 20초에서 약 2초로 줄이는 실사용 성능 개선이다.

## 6. Build cache 잔여물 확인 및 cleanup

이미지 태그 삭제는 이미지 태그/layer 삭제 문제를 검증하는 것이고, BuildKit build cache는 별도 영역이다. 최종 cleanup 후에도 테스트 빌드가 만든 cache가 남았다.

```text
Build cache usage before builder prune: 806.1MB
```

테스트 부산물 cleanup 목적으로 `docker builder prune --force`를 실행했고, 다음이 회수됐다.

```text
Total: 806.1MB
```

이 부분은 dockviz Disk Usage의 Build Cache 행이 필요한 이유를 보여준다. 이미지 태그를 지웠는데도 cache가 남아 있을 수 있고, 이때는 Images 삭제가 아니라 Build Cache prune이 맞는 조치다.

## 7. 최종 cleanup 확인

label 기반으로 만든 테스트 컨테이너, volume, image는 남아 있지 않았다.

```text
docker ps -a --filter label=dockviz.validation
docker volume ls --filter label=dockviz.validation
docker image ls --filter label=dockviz.validation
```

세 명령 모두 빈 결과를 반환했다.

최종 `docker system df -v` 기준:

```text
Containers:    none
Local Volumes: none
Build cache:   0B
```

base image인 `busybox:1.36`만 남았다. 이 이미지는 6.76MB이고, 검증 workload 실행에 사용된 공통 base image다.

## 8. UI/문서에 연결되는 해석

관련 화면:

| 기능 | 화면 |
|---|---|
| 실시간 컨테이너 resource summary | ![Containers panel](images/dockviz-containers.svg) |
| 컨테이너별 상세 CPU/MEM/p95/peak/trend | ![Container detail](images/dockviz-container-detail.svg) |
| 문제 severity와 resource pressure | ![Problems panel](images/dockviz-problems.svg) |
| reclaimable/Host Storage 분리 | ![Disk Usage panel](images/dockviz-disk-usage.svg) |
| Build Cache prune 확인 | ![Build Cache prune confirmation](images/dockviz-confirm-build-cache.svg) |
| Local Volumes prune 확인 | ![Local Volumes prune confirmation](images/dockviz-confirm-volumes.svg) |

이번 검증에서 dockviz가 Docker CLI보다 의미를 갖는 지점은 다음으로 정리된다.

1. `docker volume prune` 기본값 때문에 놓치는 named volume을 UI 표시와 실제 prune 동작이 일치하도록 처리한다.
2. `docker system df`가 설명하지 못하는 Windows Docker Desktop VHDX allocation을 Docker reclaimable과 분리해 보여준다.
3. 이미지 태그 삭제를 안전하게 만들어, 태그 하나를 지우려다 다른 태그/layer까지 날리는 위험을 줄인다.
4. 컨테이너별 stats 조회를 병렬화해 TUI refresh가 컨테이너 수에 비례해서 느려지는 문제를 줄인다.
5. 이미지 삭제 후에도 남는 BuildKit cache를 별도 reclaim 대상임을 보여준다.

## 9. 남은 한계

이번 검증으로 확인된 한계도 명확하다.

- VHDX compact는 dockviz가 자동으로 실행하면 안 된다. Docker Desktop/WSL 상태, 관리자 권한, 실행 중인 VM 상태에 따라 위험도가 있으므로 read-only 진단과 안내가 맞다.
- Build cache prune은 개발자의 rebuild 시간을 늘릴 수 있으므로 confirmation이 필요하다.
- Local Volumes prune은 실제 애플리케이션 데이터 손실 가능성이 있으므로 가장 강한 경고가 필요하다.
- stats 병렬화는 daemon/API 응답 지연을 줄이는 것이 아니라, 여러 컨테이너 조회의 wall-clock latency를 줄이는 방식이다.

결론: 이 프로젝트의 의미는 “Docker 명령어를 대신 실행하는 TUI”가 아니라, daemon에서 흩어진 storage/resource/problem 신호를 한 화면에 모으고, 실제로 회수 가능한 대상과 단순 진단 대상을 분리하며, 위험한 조치에는 명확한 confirmation을 거는 운영 보조 도구라는 점이다.
