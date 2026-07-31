# dockviz 오픈소스 활용과 성능 개선 전략

이 문서는 dockviz-cli가 어떤 오픈소스를 사용했고, 그중 무엇이 실제 성능 개선에 직접 기여했는지 정리한다. 핵심 기준은 “오픈소스를 붙였다”가 아니라 “사용자가 TUI에서 느끼는 지연, 반복 명령 비용, 문제 원인 파악 비용을 줄였는가”다.

## 1. 결론

현재 성능 개선에 가장 직접적으로 기여한 조합은 다음이다.

| 오픈소스/기술 | 사용 위치 | 성능 개선 기여 |
|---|---|---|
| Go goroutine / `sync.WaitGroup` | `internal/tui/model.go` | 컨테이너 stats 조회를 순차에서 병렬로 전환 |
| Bubble Tea `tea.Cmd` | `internal/tui/model.go`, `internal/tui/update.go` | Docker API 조회를 TUI 이벤트 루프 밖에서 실행해 화면 갱신 구조 유지 |
| Docker Go SDK | `internal/docker/*` | Docker CLI text parsing 대신 daemon API를 직접 호출 |
| compose-go | `internal/compose/*` | `docker compose` 명령을 shell out하지 않고 Compose 파일을 in-process로 해석 |
| Go 표준 라이브러리 benchmark helper | `scenarios/stats_parallel_benchmark.go` | 실제 dockviz 코드 경로로 순차/병렬 stats 성능 검증 |

실측 기준으로 가장 큰 개선은 stats refresh다.

```text
실행 ID: dockviz-core-20260730142414
컨테이너 수: 12
반복 횟수: 5

순차 stats 평균: 19.730s
병렬 stats 평균:  2.058s
개선 비율:        9.585x
```

관련 상세 검증 리포트는 [core-fixes-validation-report.ko.md](core-fixes-validation-report.ko.md)에 기록되어 있다.

## 2. 성능 문제를 어떻게 정의했는가

dockviz의 병목은 단순히 “렌더링이 느리다”가 아니었다. 실제 문제는 Docker daemon을 대상으로 여러 종류의 정보를 매번 조합해야 한다는 점이다.

TUI 한 번의 refresh에서 필요한 정보:

1. 컨테이너 목록
2. 이미지 목록
3. 실행 중인 컨테이너별 CPU/MEM stats
4. 문제 패널용 최근 event/resource history
5. Disk Usage 패널 진입 시 Docker df, volume, image, build cache, log size 정보
6. Compose 파일이 있으면 service/dependency/network/volume context

이 중 가장 위험한 병목은 컨테이너별 stats 조회다. 컨테이너가 12개라면 stats API도 12번 필요하다. 순차 처리하면 하나의 느린 API 호출이 다음 호출을 계속 막는다.

따라서 성능 개선 방향은 다음이었다.

- Docker API 호출은 TUI 화면 렌더링과 분리한다.
- 서로 독립적인 Docker API 호출은 병렬화한다.
- 매 refresh마다 필요하지 않은 무거운 조회는 해당 패널에서만 수행한다.
- CLI 출력 파싱 대신 Docker SDK의 typed API를 사용한다.
- 검증은 PowerShell job이나 외부 CLI가 아니라 실제 dockviz 코드 경로와 같은 Go SDK 호출로 한다.

## 3. Bubble Tea: 비동기 작업을 TUI 구조 안에 넣기

사용 오픈소스:

- [`github.com/charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea)

Bubble Tea는 Elm Architecture 기반 TUI framework다. dockviz에서는 화면 상태를 `Model`에 두고, 사용자 입력/타이머/Docker 조회 결과를 message로 받아 갱신한다.

성능상 중요한 점은 `tea.Cmd`다. `tea.Cmd`는 “지금 당장 화면 렌더링 안에서 무거운 작업을 하지 말고, 밖에서 실행한 뒤 결과를 message로 돌려보내는 구조”를 만든다.

현재 핵심 경로:

```text
internal/tui/model.go
└─ fetchDataCmd(dc docker.DockerClient) tea.Cmd
   ├─ containers 조회
   ├─ images 조회
   └─ running container stats 병렬 조회
```

Bubble Tea를 쓰지 않았다면 다음 위험이 커진다.

- Docker API 호출이 UI loop를 직접 막는다.
- refresh 중 키 입력 반응성이 떨어진다.
- logs/events streaming, stats refresh, disk usage 조회가 서로 엉키기 쉽다.
- 동시성 코드를 아무 곳에나 넣어 Model 상태 race가 생기기 쉽다.

dockviz에서는 Bubble Tea의 단일 Update 흐름을 유지하고, Docker 조회는 `tea.Cmd` 내부에서 수행한다. 즉, 병렬 조회는 background에서 수행하되 최종 Model 변경은 message 처리 시점에 모은다.

이 방식은 “성능”과 “상태 안정성”을 같이 잡는 구조다.

## 4. Go goroutine: 컨테이너별 stats 병렬화

직접 성능 개선에 가장 크게 기여한 부분이다.

구현 위치:

- `internal/tui/model.go`
- 함수: `fetchDataCmd`

현재 흐름:

```go
var statsMu sync.Mutex
var statsWg sync.WaitGroup
for i, c := range containers {
    if c.Status != "running" {
        continue
    }
    statsWg.Add(1)
    i, c := i, c
    go func() {
        defer statsWg.Done()
        cpu, mem, err := dc.FetchStats(c.ID)
        if err != nil {
            return
        }
        statsMu.Lock()
        containers[i].CPUPerc = cpu
        containers[i].MemMB = mem
        statsMu.Unlock()
    }()
}
statsWg.Wait()
```

핵심은 컨테이너별 `FetchStats`가 서로 독립적이라는 점이다.

순차 방식:

```text
stats(container-1)
  -> stats(container-2)
    -> stats(container-3)
      -> ...
```

병렬 방식:

```text
stats(container-1) ┐
stats(container-2) ├─ WaitGroup으로 완료 대기
stats(container-3) ┘
```

이 구조에서는 가장 느린 API 호출 시간이 전체 대기 시간의 상한에 가까워진다. 반대로 순차 방식에서는 모든 컨테이너 API 호출 시간이 누적된다.

실제 측정 결과:

| run | sequential | parallel | speedup |
|---:|---:|---:|---:|
| 1 | 19.494s | 2.091s | 9.325x |
| 2 | 20.092s | 2.436s | 8.248x |
| 3 | 19.580s | 1.936s | 10.113x |
| 4 | 21.141s | 1.879s | 11.248x |
| 5 | 18.343s | 1.949s | 9.409x |

평균:

```text
19.730s -> 2.058s
9.585x speedup
```

이 수치는 “보이는 정보가 많아졌다” 정도가 아니라, 컨테이너 12개 환경에서 refresh가 사실상 멈춘 것처럼 보이는 상태를 약 2초 수준으로 줄인 것이다.

## 5. Docker Go SDK: CLI wrapper가 아니라 daemon client로 동작

사용 오픈소스:

- [`github.com/docker/docker`](https://github.com/moby/moby/tree/master/client)

사용 위치:

- `internal/docker/client.go`
- `internal/docker/containers.go`
- `internal/docker/images.go`
- `internal/docker/diskusage.go`
- `internal/docker/events.go`
- `internal/docker/logs.go`

dockviz는 일반적인 화면 조회에서 `docker ps`, `docker stats`, `docker image ls` 같은 CLI 명령을 반복 실행하지 않는다. Docker Go SDK로 daemon API를 직접 호출한다.

성능상 이점:

1. 매 refresh마다 Docker CLI process를 새로 띄우는 비용이 없다.
2. CLI text output을 파싱하지 않아도 된다.
3. Docker object를 typed struct로 받아 후속 계산이 단순해진다.
4. event/log stream처럼 지속 연결이 필요한 기능을 구조적으로 다루기 쉽다.
5. `ContainerStats`, `ContainerList`, `ImageList`, `DiskUsage` 같은 API를 코드에서 조합할 수 있다.

예를 들어 stats는 다음 경로로 들어온다.

```text
internal/tui/model.go
└─ dc.FetchStats(c.ID)
   └─ internal/docker/containers.go
      └─ c.cli.ContainerStats(context.Background(), id, false)
```

이 방식은 Docker CLI를 단순히 감싸는 것과 다르다. CLI wrapper라면 성능 개선은 “명령을 몇 번 덜 호출한다” 수준에 머물 가능성이 크다. SDK를 쓰면 API 호출 단위를 직접 제어할 수 있어서, 어떤 호출을 병렬화하고 어떤 호출을 panel 진입 시점으로 늦출지 결정할 수 있다.

## 6. Disk Usage 성능 전략: 무거운 조회는 필요할 때만

구현 위치:

- `internal/tui/model.go`
- 함수: `fetchDiskUsageCmd`

Disk Usage는 이미지, 컨테이너, 볼륨, build cache, log size, host storage까지 엮인다. 이 조회를 2초마다 항상 수행하면 컨테이너 화면이나 Problems 화면에서도 불필요한 daemon/file-system 비용이 발생한다.

현재 구조:

```go
func fetchDiskUsageCmd(dc docker.DockerClient) tea.Cmd {
    return func() tea.Msg {
        info, err := dc.DiskUsage()
        return diskUsageMsg{info: info, err: err}
    }
}
```

그리고 주석 그대로 Disk Usage 패널이 active일 때만 호출한다.

성능상 의미:

- Containers 패널 refresh는 컨테이너 resource 정보에 집중한다.
- Disk Usage 패널에 들어갈 때만 storage breakdown을 계산한다.
- build cache/log/host storage처럼 느릴 수 있는 조회가 일반 refresh를 오염시키지 않는다.

이건 “병렬화”가 아니라 “조회 시점 분리”에 의한 성능 개선이다.

## 7. compose-go: 성능보다 shell-out 제거와 진단 품질 개선

사용 오픈소스:

- [`github.com/compose-spec/compose-go/v2`](https://github.com/compose-spec/compose-go)

사용 위치:

- `internal/compose/context.go`
- `internal/tui/compose_context.go`

compose-go는 stats refresh 자체를 빠르게 만드는 핵심 병렬화 도구는 아니다. 여기서 과장하면 안 된다. compose-go의 주된 기여는 다음이다.

1. `docker compose config` 같은 외부 명령을 실행하지 않고 Compose 파일을 직접 해석한다.
2. Compose service, dependency, network, configured port/volume 정보를 daemon label과 매칭한다.
3. Problems detail에서 문제가 난 컨테이너가 어떤 service이고 어떤 dependent를 갖는지 보여준다.
4. 사용자가 `docker compose ps`, `docker inspect`, Compose YAML을 번갈아 열어보는 시간을 줄인다.

즉 compose-go의 성능 개선은 CPU benchmark 수치보다는 “문제 원인 파악 시간 단축”에 가깝다.

예시:

```text
postgres-db container 문제 발생
└─ compose-go context
   ├─ service: postgres
   ├─ dependent services
   ├─ configured volume
   ├─ configured ports
   └─ source compose file
```

이 정보가 없으면 사용자는 CLI를 여러 번 조합해야 한다.

```bash
docker ps
docker inspect <container>
docker compose ps
docker compose config
docker stats
```

compose-go를 쓰면 dockviz가 live daemon data 위에 Compose context를 얹어서 보여준다. 이것은 runtime stats를 빠르게 하는 최적화는 아니지만, 운영자가 문제를 좁히는 데 드는 command round-trip을 줄이는 개선이다.

## 8. 검증 방식도 오픈소스 구조에 맞췄다

처음에는 PowerShell `Start-Job`으로 stats 병렬 측정을 시도할 수 있다. 하지만 이 방식은 dockviz 실제 구현과 다르다. PowerShell job은 process/job startup overhead가 크고, Docker CLI를 다시 호출하므로 Go goroutine + Docker SDK 구조를 검증하지 못한다.

그래서 별도 Go helper를 추가했다.

파일:

- `scenarios/stats_parallel_benchmark.go`

이 helper는 dockviz와 같은 경로를 사용한다.

```text
stats_parallel_benchmark.go
└─ internal/docker.NewClient("")
   └─ FetchStats(container)
```

비교 대상:

1. 같은 Docker SDK client로 `FetchStats` 순차 호출
2. 같은 Docker SDK client로 `FetchStats` goroutine 병렬 호출

이렇게 해야 “오픈소스를 사용한 실제 구현이 빨라졌는가”를 제대로 검증할 수 있다.

## 9. 오픈소스별 역할 구분

### Bubble Tea

역할:

- TUI event loop
- `tea.Cmd` 기반 background task
- keyboard/timer/message 처리 구조

성능 기여:

- 무거운 Docker 조회를 View rendering에서 분리
- logs/events/stats 같은 비동기 입력을 안정적으로 처리
- Model 업데이트를 중앙화해 race 가능성을 줄임

한계:

- Docker API 자체를 빠르게 만들지는 않는다.
- 잘못 쓰면 `Update` 안에서 오래 걸리는 작업을 실행해 UI가 멈출 수 있다.

### Go goroutine / sync

역할:

- 독립적인 Docker API 호출 병렬화
- `WaitGroup`으로 결과 수집
- `Mutex`로 shared slice 업데이트 보호

성능 기여:

- stats refresh wall-clock time 감소
- 컨테이너 수 증가에 따른 선형 지연 완화

한계:

- daemon이 과부하 상태면 병렬 요청이 오히려 부담이 될 수 있다.
- 향후 컨테이너 수가 수십~수백 개로 커지면 worker pool/concurrency limit이 필요하다.

### Docker Go SDK

역할:

- daemon API 직접 연결
- typed Docker object 조회
- stats/events/logs/disk usage 통합

성능 기여:

- CLI process spawn 제거
- text parsing 제거
- API 호출 단위 제어 가능

한계:

- Docker Desktop/remote daemon 환경 차이를 직접 처리해야 한다.
- SDK API 변화와 daemon API version 차이를 의식해야 한다.

### compose-go

역할:

- Compose YAML 해석
- service/dependency/network/volume context 생성
- live container label과 Compose model 매칭

성능 기여:

- shell-out 제거
- 문제 분석 command round-trip 감소
- Problems/Detail 화면에서 원인 범위 축소

한계:

- CPU/MEM stats 자체를 빠르게 하지는 않는다.
- Compose 파일이 실제 running container와 어긋나면 context가 부정확할 수 있다.

## 10. 앞으로의 성능 개선 방향

현재 병렬화는 컨테이너 12개에서 명확한 효과가 있었다. 다음 단계는 “더 많이 병렬화”가 아니라 “daemon에 무리가 가지 않는 선에서 제어 가능한 병렬화”다.

우선순위:

1. stats 조회 concurrency limit 추가
   - 예: 동시에 최대 8개 또는 16개만 `FetchStats`
   - 컨테이너 수가 많을 때 daemon burst 방지

2. stats cache TTL 도입
   - 아주 최근 sample이 있으면 즉시 표시
   - background에서 최신 sample 갱신
   - UI는 빈 값 대신 last-known-good 값을 유지

3. Disk Usage incremental refresh
   - panel 진입 시 전체 조회
   - 이후에는 build cache/log/host storage처럼 느린 항목을 서로 다른 주기로 갱신

4. event 기반 refresh trigger
   - container start/die/oom/kill event 발생 시 관련 영역만 빠르게 갱신
   - 무조건 2초 polling보다 불필요한 daemon 호출 감소

5. compose-go 결과 cache
   - Compose 파일 mtime/hash가 바뀌지 않으면 재파싱하지 않음
   - live daemon label 매칭만 갱신

6. Problems 계산 cost 분리
   - resource history 기반 severity 계산과 storage offender 계산을 분리
   - storage offender는 Disk Usage 데이터가 최신일 때만 갱신

## 11. “AI + 오픈소스” 관점의 정리

현재 dockviz runtime 안에 AI를 넣은 것은 아니다. 즉 사용자의 Docker daemon 정보가 AI API로 전달되는 구조가 아니다.

이번 개발에서 AI가 한 역할은 다음에 가깝다.

- 병목 가설 정리
- Docker API 동작 차이 분석
- 오픈소스 후보 평가
- 검증 시나리오 설계
- 측정 결과 해석
- 문서화

반대로 오픈소스는 runtime 구현의 핵심이다.

- Bubble Tea: TUI architecture
- Docker SDK: daemon integration
- compose-go: Compose model parsing
- Go runtime: concurrency

따라서 현재 방식은 “AI가 제품 안에서 최적화를 수행하는 구조”가 아니라, AI를 개발/검증 파트너로 쓰고 오픈소스 런타임 구성요소로 실제 기능과 성능을 구현한 방식이다.

## 12. 최종 판단

성능 개선 관점에서 가장 의미 있는 선택은 Docker CLI wrapper를 피하고, Docker Go SDK + Bubble Tea `tea.Cmd` + Go goroutine 조합으로 간 것이다.

이 조합 덕분에 dockviz는 다음 특성을 갖는다.

- Docker daemon과 직접 연결한다.
- 독립적인 API 호출을 병렬화할 수 있다.
- TUI event loop를 block하지 않는다.
- Compose context를 별도 CLI 호출 없이 얹을 수 있다.
- 실제 대용량 fixture로 개선 폭을 검증할 수 있다.

현재 검증된 가장 강한 수치는 12개 컨테이너 stats refresh에서 `19.730s -> 2.058s`, `9.585x` 개선이다. 이 수치는 dockviz가 단순 표시 도구가 아니라, 컨테이너 수가 늘어나는 실제 환경에서 refresh latency를 줄이는 구조적 개선을 갖고 있음을 보여준다.
