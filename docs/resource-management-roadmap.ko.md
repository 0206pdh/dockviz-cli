# CPU/MEM 관리 개발 로드맵

`dockviz`의 CPU/MEM 기능은 Docker 명령어를 단순히 감싸는 방향이 아니라,
짧은 시간 안에 “어떤 컨테이너가 왜 위험한지”를 판단하는 운영용 대시보드로
확장한다.

## 하지 않을 것

- 컨테이너 `exec`, start/stop/restart 같은 범용 조작 기능을 다시 넣지 않는다.
- CPU/MEM이 높다는 이유만으로 자동 kill/restart하지 않는다.
- 단일 시점 `docker stats` 복제에 머무르지 않는다.

## Phase 1 — Resource Snapshot

상태: 구현 완료

- Containers 패널에 현재 CPU/MEM뿐 아니라 CPU p95, MEM p95를 표시한다.
- Detail 화면에 avg, p95, peak, trend를 표시한다.
- Docker inspect 기반으로 CPU hard limit과 memory hard limit을 표시한다.
- limit 정보가 없거나 확인 불가한 경우를 구분한다.

목적:

- `docker stats`보다 짧은 시간 창의 추세를 더 빨리 판단한다.
- “현재 값만 우연히 높은 상태”와 “지속적으로 높은 상태”를 구분할 수 있게 한다.

## Phase 2 — Resource Problem Detection

상태: 구현 완료

- Problems 패널이 Docker event뿐 아니라 CPU/MEM history도 평가한다.
- 최근 CPU sample을 `Elevated CPU`, `High CPU`, `CPU saturated`로 단계화한다.
- memory current/p95가 configured memory limit에 가까우면 `Memory pressure`, 초과하면 `Memory over limit`로 표시한다.
- memory sample이 의미 있게 증가하면 `Memory growth`로 표시하고 limit에 가까우면 critical로 올린다.
- running 컨테이너에 CPU/MEM hard limit이 전혀 없으면 사용량에 따라 `Info` 또는 `Warning`으로 표시한다.
- CPU는 거의 쓰지 않지만 memory를 크게 잡는 `Idle memory`를 표시한다.
- compose project 안에서 한 컨테이너가 CPU 대부분을 점유하면 `Noisy neighbor`로 표시한다.
- Disk Usage가 로드되면 large logs, build cache, unused images, stopped-container layers, unattached volumes, Docker Desktop host-storage gap도 Problems에 올린다.

목적:

- 사용자가 직접 `docker stats`, `docker inspect`, event 로그를 번갈아 보지 않아도
  위험 컨테이너와 storage offender를 한 패널에서 우선순위대로 찾게 한다.

## Phase 3 — Compose/Project Resource View

상태: 구현 완료

- `compose-go`로 Compose 파일을 읽고, `com.docker.compose.project`, `com.docker.compose.service` label과 매칭한다.
- Containers 패널 상단에 project 단위 CPU/MEM 합계와 top offender를 표시한다.
- Detail 화면에 compose project/service 정보를 표시한다.
- Detail 화면에 depends_on, dependent service, network, configured volume, Compose file source를 표시한다.
- 같은 service replica가 여러 개인 경우 service 단위로 합산하는 화면은 다음 refinement로 둔다.

목적:

- 실제 사용자는 컨테이너 ID보다 compose project/service 기준으로 문제를 판단한다.
- `docker compose ps`와 `docker stats`를 수동으로 조합해야 하는 부분을 줄인다.

## Phase 4 — Recommendation Layer

상태: 구현 완료

- `High CPU`가 지속되면 CPU limit 설정 여부와 최근 p95를 근거로 추천 문구를 제공한다.
- `Memory pressure`가 발생하면 current/p95/limit 비율을 근거로 memory limit 조정 또는 leak 확인을 추천한다.
- `No resource limits`는 compose YAML 예시 형태로 `cpus`, `mem_limit` 후보를 제안한다.
- Problems 패널에서 `[Enter]`를 누르면 문제 detail과 recommendation을 표시한다.
- 추천은 read-only이며, 사용자가 명시적으로 요청하기 전까지 파일을 수정하지 않는다.

목적:

- 단순 경고가 아니라 바로 조치 가능한 근거를 제공한다.

## Phase 5 — Scenario Benchmarks

상태: 구현 완료

- CPU hog, memory pressure, memory growth, no-limit service를 재현하는 시나리오를 제공한다.
- 시나리오가 sample CSV와 expected problem summary JSON을 기록한다.
- Docker CLI로 같은 문제를 수동 확인하는 절차와 비교한다.

목적:

- “이 도구를 쓰면 무엇이 더 빨라지는가”를 정량화한다.
