# TODOS

엔지니어링 리뷰(2026-03-30) 및 디자인 문서에서 연기된 항목들.

---

## T-001: ContainerStats 스트리밍 방식 전환 (stream=true)

**What:** 2초마다 N+1 poll(컨테이너마다 FetchStats 호출) 대신 Docker 데몬이 push하는 `ContainerStats(stream=true)` 방식으로 전환.

**Why:** 폴링 지연 없는 실시간 CPU/MEM 통계 업데이트 및 네트워크 부하 감소.

**Pros:** 상태 변화 즉시 반영, 2초 tick 대기 없음.

**Cons:** 고루틴 복잡도 증가, Bubble Tea 메시지 통합 필요.

**Context:** 현재 계획은 FetchStats를 병렬 고루틴으로 호출하는 것(N+1 → 병렬). 스트리밍은 추가 개선 옵션. v0.3.0에서 병렬 고루틴 방식으로 충분한지 평가 후 결정.

**Depends on:** FetchStats 병렬 고루틴 구현(v0.2.0 버그 수정) 완료 후.

---

## T-002: Events 탭 필터링 UI

**What:** 이벤트 타입(die, restart, start 등) 또는 컨테이너 이름으로 Events 탭 필터링.

**Why:** 컨테이너가 많을수록 Events 탭에 노이즈 증가. 장애 디버깅 시 `die`, `restart` 이벤트만 보고 싶은 경우.

**Pros:** DevOps 엔지니어 사용성 향상, 온콜 시 핵심 이벤트 집중 가능.

**Cons:** UX 설계 필요(필터 입력 오버레이 또는 토글 키), v0.2.0 범위 외.

**Context:** 디자인 문서(`docs/design-v0.2.0-failure-propagation.md`)에서 v0.3.0으로 명시적 연기.

**Depends on:** v0.2.0 Events 스트리밍(Init 시작) 완료.

---

## T-003: `--demo` 크래시 시나리오 시뮬레이션

**What:** 데모 모드에서 컨테이너 크래시(die → restart 사이클) 애니메이션을 이벤트 + 토폴로지 색상 변화와 함께 시뮬레이션.

**Why:** Docker 환경 없이도 실시간 토폴로지와 이벤트 연동 기능을 스크린샷 하나로 증명 가능.

**Pros:** 포트폴리오 데모 시 핵심 기능을 라이브 Docker 없이 시연 가능.

**Cons:** demo.go 코드 복잡도 증가, 타이머 기반 이벤트 생성 필요.

**Context:** 디자인 문서에서 v0.3.0으로 연기. v0.2.0 ContainerState + 토폴로지 split view 구현 후에야 의미 있는 시뮬레이션 가능.

**Depends on:** v0.2.0 ContainerState + 그래프 split view 완료.
