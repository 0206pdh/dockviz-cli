# dockviz max-safe 대용량 성능 측정 리포트

측정일: 2026-07-29 (KST)

## 1. 측정 목적

이번 테스트의 목적은 작은 synthetic workload가 아니라, 실제 노트북의 Docker 저장소를 수 GiB 단위로 압박했을 때 dockviz가 다음을 한 화면에서 보여줄 수 있는지 검증하는 것이다.

- 어떤 Docker category가 실제 공간을 차지하는가
- 실행 중인 컨테이너와 미사용 리소스를 구분할 수 있는가
- 컨테이너가 삭제된 뒤에도 남는 reclaimable volume을 발견할 수 있는가
- 대용량 저장소 부하와 재시작 문제를 동시에 관측할 수 있는가
- 테스트 종료 후 Docker와 호스트 디스크를 실제로 원복할 수 있는가

현재 문서에는 최신 max-safe 측정 결과만 남겼다.

## 2. 테스트 전 상태

작업 폴더가 위치한 C: 드라이브와 Docker daemon을 읽기 전용으로 확인했다.

```text
C: free space       28.16 GB
Docker client       28.5.1
Docker server       28.5.1
Storage driver      overlayfs
Docker images       0
Docker containers   0
Docker volumes      0
Build cache         0 B
```

## 3. 안전한 최대 용량 계산

디스크를 문자 그대로 100% 채우면 Windows, Docker Desktop, 테스트 프로세스가 함께 멈출 수 있으므로 전체 여유 공간을 사용하지 않았다.

실행 옵션:

```powershell
powershell -ExecutionPolicy Bypass `
  -File .\scenarios\run-dockviz-performance.ps1 `
  -RunLabel max-safe `
  -UseMaxSafeStorage `
  -StorageReserveGB 12 `
  -DurationSeconds 20 `
  -SampleIntervalSeconds 2 `
  -StorageReadyTimeoutSeconds 1800
```

계산 결과:

```text
host free before    28 GB  # 스크립트가 안전하게 내림 처리한 값
reserved space      12 GB
payload selected    16 GB
```

실제 payload는 Docker volume에 `/dev/urandom` 데이터를 기록했다. 따라서 단순히 빈 파일을 만든 것이 아니라 Docker 저장소가 실제 블록을 할당하도록 했다.

## 4. 시나리오 구성

동시에 다음 리소스를 만들었다.

| 리소스 | 역할 |
|---|---|
| CPU container | CPU loop로 지속적인 CPU 부하 생성 |
| Memory container | 64 MiB 파일을 기록하고 유지 |
| Logs container | 20,000줄 로그 출력 |
| Restart container | 2초마다 exit 42, `on-failure:20` 재시작 |
| Storage container | 16 GiB random payload를 Docker volume에 기록 |

시나리오 ID는 `dockviz-perf-20260729061036`이다.

대용량 random write와 Docker Desktop 반영을 포함한 전체 실행 시간은 약 8분 24초였다. 지정한 workload 관찰 시간은 20초다.

## 5. Docker Disk Usage 측정 결과

### Workload 실행 중

```text
TYPE            TOTAL  ACTIVE  SIZE      RECLAIMABLE
Images          1      1       74.05MB   67.29MB (90%)
Containers      5      4       67.14MB  4.096kB (0%)
Local Volumes   1      1       17.18GB  0B (0%)
Build Cache     0      0       0B        0B
```

이 시점에서 dockviz의 Disk Usage 패널은 volume이 약 17 GiB를 차지하고 있고 아직 active 상태이므로 즉시 prune 대상으로 보면 안 된다는 것을 보여준다.

### Storage container 제거 후

storage container만 제거하고 volume은 남겨 둔 상태에서 다시 측정했다.

```text
TYPE            TOTAL  ACTIVE  SIZE      RECLAIMABLE
Images          1      1       74.01MB   67.25MB (90%)
Containers      4      3       67.13MB  4.096kB (0%)
Local Volumes   1      0       17.18GB  17.18GB (100%)
Build Cache     0      0       0B        0B
```

이것이 dockviz를 사용하는 핵심 이유다. 컨테이너 목록만 보면 이미 컨테이너는 삭제됐지만, Disk Usage에서는 17.18GB volume이 남아 있고 100% reclaimable이라는 사실을 즉시 확인할 수 있다.

### 측정된 runtime 지표

```text
CPU container 평균 CPU       94.10%
CPU container CPU p95        99.76%
Memory container memory p95  0.98MB
Log output                   0.50MB
Restart container restart    20회
```

Restart container는 Problems 패널의 restart-loop 조건을 충분히 충족했다. CPU·메모리·재시작 문제와 17 GiB 저장소 압박이 같은 실행에서 함께 발생한 것이다.

## 6. 정리 및 호스트 디스크 복구

테스트가 끝난 뒤 전역 prune을 실행하지 않고, `dockviz.scenario` 라벨이 붙은 테스트 리소스만 제거했다.

```text
Docker containers   0
Docker volumes      0
Docker images       0
Build cache         0B
```

Docker Desktop은 삭제된 volume 공간을 내부적으로 회수해도 WSL2 VHDX 파일 크기를 자동으로 줄이지 않을 수 있다. 그래서 Ubuntu WSL은 건드리지 않고 Docker Desktop만 잠시 중지한 뒤 Docker 데이터 VHDX만 compact했다.

```text
docker_data.vhdx    19.46GB -> 5.47GB
C: free space       13.93GB -> 28.03GB
Docker daemon       28.5.1로 재기동 및 연결 확인
Ubuntu WSL          중지하지 않음
```

임시 CSV/JSON 결과 폴더와 테스트 이미지도 삭제했다. 저장된 결과는 이 문서뿐이다.

## 7. 결론

이번 테스트는 dockviz가 단순히 `docker ps`를 예쁘게 보여주는 도구가 아니라는 점을 확인했다.

1. 17 GiB volume이 실제로 Docker 저장소를 차지하는 동안 Disk Usage에서 category별 상태를 확인할 수 있었다.
2. container 삭제 후에도 volume이 남아 100% reclaimable이 되는 상태를 확인했다.
3. CPU 과부하, 재시작 loop, 저장소 압박을 동시에 관측할 수 있었다.
4. Docker CLI의 개별 명령을 조합하지 않고 Problems와 Disk Usage를 한 화면에서 연결해 판단할 수 있었다.
5. 정리 후 Docker 내부 사용량뿐 아니라 Docker Desktop 호스트 VHDX까지 원복해야 한다는 운영상의 함정도 확인했다.

이 테스트는 애플리케이션의 HTTP latency나 RPS를 측정하는 벤치마크는 아니다. 실제 애플리케이션 성능 개선을 주장하려면 동일한 고정 workload를 `-TargetImage my-app:baseline`과 `-TargetImage my-app:improved`에 각각 실행하고, CPU/메모리와 함께 요청량·오류율·p50/p95 latency를 별도 workload generator로 수집해야 한다.
