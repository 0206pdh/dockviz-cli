# TODOS

엔지니어링 리뷰(2026-03-30) 및 디자인 문서에서 연기된 항목들.

---

## T-001: ContainerStats 스트리밍 방식 전환 (stream=true)

**What:** 2초마다 N+1 poll(컨테이너마다 FetchStats 호출) 대신 Docker 데몬이 push하는 `ContainerStats(stream=true)` 방식으로 전환.

**Why:** 폴링 지연 없는 실시간 CPU/MEM 통계 업데이트 및 네트워크 부하 감소.

**Pros:** 상태 변화 즉시 반영, 2초 tick 대기 없음.

**Cons:** 고루틴 복잡도 증가, Bubble Tea 메시지 통합 필요.

**Context:** v0.2.0에서 병렬 고루틴 방식(N+1 → 병렬) 구현 완료. 스트리밍은 추가 개선 옵션. v0.4.0에서 평가.

**Status:** 연기 → v0.4.0

---

## T-002: Events 탭 필터링 UI

**What:** 이벤트 타입(die, restart, start 등) 또는 컨테이너 이름으로 Events 탭 필터링.

**Why:** 컨테이너가 많을수록 Events 탭에 노이즈 증가. 장애 디버깅 시 `die`, `restart` 이벤트만 보고 싶은 경우.

**Pros:** DevOps 엔지니어 사용성 향상, 온콜 시 핵심 이벤트 집중 가능.

**Cons:** UX 설계 필요(필터 입력 오버레이 또는 토글 키).

**Context:** v0.3.0의 네트워크별 타임라인이 이미 암묵적 필터링을 제공. 전역 필터 UI는 Events 탭에 여전히 유용.

**Status:** 연기 → v0.4.0

---

## T-003: `--demo` 크래시 시나리오 시뮬레이션

**What:** 데모 모드에서 타이머 기반 die → restart 사이클을 구현해 토폴로지 노드 색상 변화를 녹화 없이 시연 가능하게.

**Why:** Docker 환경 없이도 실시간 토폴로지 + 이벤트 연동 기능을 GIF 하나로 증명 가능.

**Context:** v0.3.0에서 랜덤 die 이벤트에 현실적인 ExitCode/OOMKilled가 추가됨. 고정 타이밍 die → restart 사이클이 있으면 GIF 녹화에 최적.

**Status:** v0.3.0에서 부분 구현(현실적인 exit code). 스크립트 사이클 → v0.4.0

---

## T-004: 원격 환경에서도 동작하는 Container Logs 용량 측정

**What:** Disk Usage 패널의 Container Logs 카테고리는 `ContainerInspect(id).LogPath`를 로컬 파일시스템에서 직접 stat한다. dockviz와 `dockerd`가 파일시스템을 공유할 때만 동작한다(순수 리눅스, 또는 `dockerd`가 직접 도는 WSL2 distro). Docker Desktop(WSL2/Hyper-V 백엔드)이나 원격 `DOCKER_HOST`에서는 조용히 0으로 나온다. `docs/troubleshooting.md#13`, `docs/testing-container-log-disk-usage.md` 참고.

**Why:** Docker Desktop은 서버가 아닌 대다수 사용자(맥, 윈도우)의 기본 설치 경로라서, 지금은 dockviz 사용자층의 상당수에게 이 카테고리가 아예 안 보인다.

**Pros:** `ContainerLogs`로 스트리밍하며 바이트를 세는 방식은 어떤 전송 방식(원격 `DOCKER_HOST` 포함)에도 통하고, 로컬 파일시스템 접근이 아예 필요없다.

**Cons:** 크기 측정을 위해 로그 전체를 스트리밍하는 건 `stat()` 한 번보다 훨씬 비싸다. 특히 로그 히스토리가 큰 컨테이너면 더 그렇다. 패널의 정기 auto-refresh(`fetchDiskUsageCmd`)에 얹기보다, 컨테이너별로 사용자가 명시적으로 요청할 때만 도는 별도 트리거가 필요할 것.

**Context:** Container Logs 카테고리 자체와 같은 변경에서 함께 논의됨. v1에서는 의도적으로 범위 밖으로 뺐다 — 로컬 stat이 비용 없이 이 수치를 얻는 유일한 방법이고, 이 프로젝트가 명시한 주 타겟인 네이티브 리눅스 서버 운영자에게는 지금도 잘 동작한다.

**Status:** 연기

---

## T-005: 자동화된 테스트/린트 워크플로 없음

**What:** `.github/workflows/`에는 `release.yml`(main push/태그 시 빌드+배포)만 있다. PR 머지 전에 `go build`, `go vet`, `go test ./...`를 실행하는 게 아무것도 없다.

**Why:** Local Volumes 프룬 회수량 불일치 버그(`docs/troubleshooting.md#12`)가 원래 Disk Usage 패널 PR에 그대로 실려서 한 릴리스 주기 동안 아무도 모르고 지나갔다. 사실 `go test ./...`가 있었어도 이건 못 잡았을 것 — 실제 데몬과 통신하는 통합 버그인데, 진짜 Docker API를 호출하는 `Client`의 메서드들은 커버리지 0%다(테스트되는 건 사실상 `DemoClient`뿐). 그래도 최소한 `go vet`과 기존 유닛 테스트가 PR마다 자동으로 돌면, 커버되는 범위 안에서의 회귀는 누군가 로컬에서 기억해서 돌리는 것에 기대지 않고 잡을 수 있다.

**Pros:** `release.yml`이 깨진 커밋으로 이미 태그를 찍고 배포해버리기 전에, 머지 전 단계에서 빌드 실패와 유닛 테스트 회귀를 잡는다.

**Cons:** `Client`에 대한 실제 데몬 통합 테스트까지 커버하려면(T-004의 비용 문제, `docs/testing-container-log-disk-usage.md`에 적은 0% 커버리지 갭 참고) `services: docker` 같은 GitHub Actions 러너 설정이 추가로 필요하다.

**Context:** 지금까지 머지된 PR(#6~#11)은 전부 작성자가 로컬에서 `go build`/`go test`를 돌려본 뒤 push하는 데 의존했다. 메인테이너가 한 명인 지금 규모에선 그럭저럭 괜찮지만, 자동 릴리스(`release.yml`, #10에서 머지)는 `main`에 올라온 걸 즉시 배포한다 — "머지됨"과 "PyPI/APT/GitHub Releases에 배포됨" 사이에 아무 게이트가 없다.

**Status:** 연기
