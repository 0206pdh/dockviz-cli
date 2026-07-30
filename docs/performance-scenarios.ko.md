# max-safe 대용량 성능 시나리오

이 시나리오는 Docker daemon의 상태·문제·디스크 사용량을 실제 대용량 부하와 연결해 확인한다. 현재 기준 시나리오는 노트북의 여유 공간을 자동 계산해 12 GiB를 예약하고 나머지를 Docker volume payload로 사용한다.

## 만드는 부하

- CPU loop 컨테이너
- 64 MiB 메모리 컨테이너
- 20,000줄 로그 컨테이너
- 2초마다 exit 42를 내는 restart-loop 컨테이너
- max-safe random payload를 기록하는 Docker volume 컨테이너

실행 중 다른 터미널에서 `go run .`을 실행하면 Containers, Problems, Disk Usage 패널을 동시에 관찰할 수 있다.

## max-safe 실행

Docker daemon이 실행 중인 프로젝트 루트에서 실행한다.

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

`-UseMaxSafeStorage`는 작업 폴더가 있는 드라이브의 여유 공간을 계산한 뒤 예약 공간을 제외한 정수 GiB를 선택한다. 디스크를 100%까지 채우지 않는 이유는 Windows와 Docker Desktop이 계속 동작할 공간이 필요하기 때문이다.

테스트 중 dockviz에서 확인할 항목:

- Containers: CPU%, 메모리, active 상태, restart 횟수
- Problems: restart-loop와 비정상 종료 이벤트
- Disk Usage: Images, Containers, Local Volumes, Build Cache, Container Logs

## 결과 파일

`artifacts` 아래에 다음 결과가 생성된다.

- `*-summary.json`: 컨테이너별 CPU 평균/p95, 메모리 p95, restart 횟수, max-safe 계산값
- `*-samples.csv`: 샘플별 원시 측정값
- `*-before-system-df.txt`: 테스트 전 Docker 디스크 사용량
- `*-workload-system-df.txt`: 대용량 volume이 active인 상태
- `*-reclaimable-system-df.txt`: storage 컨테이너 제거 후 미사용 volume 상태
- `*-after-cleanup-system-df.txt`: 테스트 리소스 정리 후 Docker 디스크 사용량

스크립트는 전역 prune을 호출하지 않는다. `dockviz.scenario` 라벨이 붙은 테스트 컨테이너와 테스트 volume만 정리한다.

Docker Desktop에서는 Docker 내부 사용량이 0이 되어도 WSL2 VHDX 파일이 자동으로 줄지 않을 수 있다. 호스트 디스크까지 원복해야 하는 경우 Docker Desktop만 중지하고 `docker_data.vhdx`를 compact한 뒤 다시 시작해야 한다. 다른 WSL 배포판까지 중지하는 `wsl --shutdown`은 사용하지 않는다.

## 애플리케이션 성능 비교

실제 애플리케이션의 Docker-level CPU/메모리를 비교하려면 같은 고정 workload를 대상 이미지에 전달한다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scenarios\run-dockviz-performance.ps1 `
  -RunLabel app-baseline `
  -UseMaxSafeStorage `
  -TargetImage my-app:baseline `
  -TargetCommand "./run-fixed-workload.sh" `
  -DurationSeconds 60 `
  -StorageReadyTimeoutSeconds 1800
```

개선 버전에서는 `my-app:improved`로 바꾸고 동일한 command, 입력량, duration을 사용한다. HTTP 성능까지 주장하려면 요청량(RPS), 오류율, p50/p95 latency를 별도 workload generator로 수집해야 한다.

이번에 실제 실행한 결과는 [performance-results.ko.md](performance-results.ko.md)에 기록되어 있다.

## CPU/MEM health detection 시나리오

디스크 정리 효과와 별도로 CPU/MEM 관리 기능을 확인하려면
`scenarios/run-dockviz-resource-health.ps1`을 실행한다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scenarios\run-dockviz-resource-health.ps1 `
  -RunLabel resource-health `
  -DurationSeconds 30 `
  -SampleIntervalSeconds 2 `
  -OutputDirectory .\artifacts
```

이 시나리오는 compose-style label을 가진 다음 컨테이너를 만든다.

- CPU hog
- memory pressure
- memory growth
- CPU/MEM limit이 없는 컨테이너

예상 확인 지점:

- Containers 패널 상단에 project resource summary가 표시된다.
- Problems 패널에 `High CPU` 또는 `CPU saturated`, `Memory pressure` 또는 `Memory over limit`, `Memory growth`,
  `No resource limits`가 표시된다.
- Problems 패널에서 `[Enter]`를 누르면 detail과 read-only recommendation이 표시된다.

실제 smoke 결과는
[`resource-health-smoke-results.ko.md`](resource-health-smoke-results.ko.md)에 기록되어 있다.
