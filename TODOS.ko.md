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
