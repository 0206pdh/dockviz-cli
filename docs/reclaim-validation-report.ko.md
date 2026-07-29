# dockviz reclaim 검증 리포트

측정일: 2026-07-29 KST  
환경: Windows Docker Desktop WSL2, Docker client/server 28.5.1  
검증 실행 ID: `dockviz-reclaim-complete-20260729074256`

## 1. 검증 목적

이번 검증은 dockviz의 Disk Usage/Images 패널이 실제 운영에서 의미 있는
저장공간 문제를 찾는지 확인하기 위한 것이다. 특히 `docker ps`만 보거나
일회성 `docker prune`만 실행하면 놓치기 쉬운 항목을 분리해서 만들었다.

- tagged unused image: 컨테이너가 쓰지 않지만 tag가 있어서 dangling prune으로는 지워지지 않는 이미지
- dangling image: rebuild/retag 후 `<none>:<none>`으로 남은 이미지
- unused volume: 컨테이너 삭제 후 데이터만 남은 volume
- Docker Desktop VHDX: Docker 객체 삭제 후에도 Windows host 용량이 바로 회복되지 않는 영역

Build Cache는 이번 real-daemon 삭제 테스트에서 제외했다. dockviz의 Build Cache
행은 Docker API의 `BuildCachePrune(All: true)`에 연결되어 있지만, 이 테스트에서
전역 build cache를 만들고 지우면 사용자의 기존 build cache까지 건드릴 수 있다.
따라서 실제 삭제 검증은 테스트에서 만든 image/volume/container ID만 정확히
삭제하는 방식으로 제한했다.

## 2. 시작 상태

이전 실패한 준비 실행에서 pull된 `busybox:1.36`만 남아 있었다. 테스트 마지막에
이 base image도 제거했다.

```text
Images:
busybox:1.36   6.76MB   containers=0

Containers:    none
Local Volumes: none
Build Cache:   0B

C: free space:             25.78GB
docker_data.vhdx size:      7.97GB
```

## 3. 만든 fixture

### 3.1 Tagged unused image

`busybox:1.36` 컨테이너 안에 random payload를 쓰고 commit해서 tagged image를
만든 뒤 seed container를 삭제했다.

```text
dockviz-reclaim-complete-20260729074256-unused:latest
image id: bce35a9e75ec
size:     2.15GB
unique:   2.15GB
containers: 0
```

이 상태는 dockviz에서 Images 패널에는 보이지만, Disk Usage의 safe Images prune
대상에는 포함하지 않는 것이 맞다. tag가 살아 있으므로 `docker image prune`의
기본 dangling-only 동작으로는 지워지지 않는다. dockviz는 이런 공간을
`outside selected prune` 성격으로 분리해서 보여줘야 한다.

### 3.2 Dangling image

같은 tag를 두 번 commit해서 이전 image가 `<none>:<none>`으로 떨어지게 만들었다.

```text
<none>:<none>
image id: 6136f2acadc4
size:     1.62GB
unique:   1.613GB
containers: 0
```

이 항목은 Docker daemon 기준으로 명확한 reclaimable image다. dockviz Disk Usage
패널에서 Images 행의 reclaimable로 잡혀야 하고, Images prune 확인창을 통해
삭제 가능한 유형이다.

### 3.3 Unused volume

named volume에 2GiB random payload를 쓴 뒤 writer container를 삭제했다.

```text
volume: dockviz-reclaim-complete-20260729074256-volume
links:  0
size:   2.147GB
```

이 상태는 `docker ps`나 Images 패널만 보면 놓치기 쉽다. dockviz Disk Usage의
Local Volumes 행에서 `ACTIVE=0`, `RECLAIMABLE=2.147GB` 성격으로 보여야 하는
대표적인 케이스다. 실제 UI에서는 Local Volumes 확인창이 “데이터 손실 가능성”을
명시한다.

## 4. 발견된 총 정리 가능 Docker 객체

fixture 생성 직후 Docker daemon이 설명한 주요 정리 후보는 다음과 같다.

| 유형 | 식별 방식 | 크기 | dockviz에서의 위치 | 삭제 방식 |
|---|---:|---:|---|---|
| Tagged unused image | tag 있음, containers=0 | 2.15GB | Images panel, Disk Usage note | Images panel에서 `d`, 또는 `docker rmi <tag>` |
| Dangling image | `<none>:<none>` | 1.613GB unique | Disk Usage Images reclaimable | Disk Usage Images에서 `d`, 또는 `docker rmi <image-id>` |
| Unused volume | links=0 | 2.147GB | Disk Usage Local Volumes | Disk Usage Local Volumes에서 `d`, 또는 `docker volume rm <name>` |

합산하면 테스트가 만든 Docker 객체 기준으로 약 5.91GB를 찾아낼 수 있었다.
여기에 replacement tag와 base image 같은 잔여 fixture를 마지막 cleanup에서
추가로 제거했다.

## 5. 삭제 검증

### 5.1 Tagged unused image 삭제

실행한 동작:

```powershell
docker rmi dockviz-reclaim-complete-20260729074256-unused:latest
```

삭제 후 `docker system df -v`에서 2.15GB tagged unused image가 사라졌다. 이
공간은 dangling prune으로는 없어지지 않는 유형이므로, dockviz Images 패널에서
사용자가 직접 보고 지우는 UX가 필요하다.

남은 항목:

```text
<none>:<none> dangling image                     1.62GB
dockviz-reclaim...-volume unused volume          2.147GB
replacement dangling tag                         6.8MB
busybox base image                               6.76MB
```

### 5.2 Dangling image 삭제

실행한 동작:

```powershell
docker rmi sha256:6136f2acadc4aa5ff2e5d02875545faba026b45596be0fee2b506806e0d603c1
```

삭제 후 `<none>:<none>` 1.62GB image가 사라졌다. 이 유형은 dockviz의 Disk Usage
Images 행에서 reclaimable로 보여주는 것이 맞다.

### 5.3 Unused volume 삭제

실행한 동작:

```powershell
docker volume rm dockviz-reclaim-complete-20260729074256-volume
```

삭제 후 Local Volumes 섹션이 비었다. 이 유형은 실제 애플리케이션 데이터가 들어
있을 수 있으므로 dockviz의 Local Volumes 확인창 경고가 필요하다.

### 5.4 최종 cleanup

replacement image와 `busybox:1.36`까지 제거한 뒤 최종 상태는 다음과 같았다.

```text
Images          0         0         0B        0B
Containers      0         0         0B        0B
Local Volumes   0         0         0B        0B
Build Cache     0         0         0B        0B
```

Docker daemon 기준 reclaim은 완료됐다.

## 6. Docker Desktop VHDX 결과

이번 테스트에서 가장 중요한 운영상 결론은 Docker 객체 삭제와 Windows host 용량
회복이 같은 일이 아니라는 점이다.

| 시점 | C: free | `docker_data.vhdx` |
|---|---:|---:|
| 시작 | 25.78GB | 7.97GB |
| tagged unused image 생성 후 | 25.06GB | 8.69GB |
| dangling image 생성 후 | 24.37GB | 9.38GB |
| unused volume 생성 후 | 18.84GB | 9.38GB |
| Docker 객체 삭제 직후 | 18.84GB | 9.38GB |
| 이후 재확인 | 23.75GB | 9.38GB |

해석:

- `docker system df`가 0B로 돌아와도 `docker_data.vhdx`는 자동으로 작아지지 않는다.
- Docker가 내부 객체를 삭제하면 Docker Desktop VM 안에서는 재사용 가능한 공간이 생긴다.
- Windows C: 드라이브 입장에서는 VHDX compaction 전까지 파일 할당이 남을 수 있다.
- 따라서 dockviz는 Docker reclaimable과 Host Storage VHDX를 분리해서 보여줘야 한다.

이번 실행에서는 정책상 `diskpart compact vdisk` 실행이 차단되어 compact까지
자동으로 수행하지 못했다. 이전 max-safe 테스트에서는 같은 VHDX를 compact했을 때
`19.46GB -> 5.47GB`로 줄어들고 C: free space가 `13.93GB -> 28.03GB`로 회복되는
것을 확인했다.

## 7. dockviz 기준 UX 결론

이번 검증으로 다음 판단이 가능해졌다.

1. Tagged unused image는 Disk Usage의 prune 숫자로만 처리하면 안 된다. tag가
   살아 있으므로 Images 패널에서 직접 보여주고 삭제해야 한다.
2. Dangling image는 Docker daemon reclaimable이므로 Disk Usage Images 행에서
   바로 삭제 가능한 후보로 보여주는 것이 맞다.
3. Unused volume은 `docker ps`에서는 보이지 않지만 수 GiB를 차지할 수 있다.
   Disk Usage Local Volumes 행과 강한 confirmation copy가 필요하다.
4. Docker 객체를 모두 지운 뒤에도 VHDX가 남는 현상은 Docker prune 실패가 아니다.
   host-side 가상 디스크 allocation 문제이며, 별도 Host Storage 섹션으로 알려야 한다.
5. “outside Docker df”는 prune 예상치가 아니라 진단 gap이다. 이 값을 보고 바로
   삭제 버튼을 제공하면 위험하고, compact/운영 안내로 연결해야 한다.

## 8. 한계

이제 dockviz가 모든 잔여 용량을 자동으로 삭제할 수 있는 것은 아니다. 의도적으로
삭제 버튼을 제공하지 않는 영역이 있다.

- tagged unused image: 사용자가 tag 의미를 확인한 뒤 Images 패널에서 삭제
- Docker Desktop VHDX: prune 대상이 아니라 compact 대상
- Build Cache: 전역 cache 삭제는 개발 workflow에 영향을 줄 수 있으므로 확인창 필요
- remote Docker host: 로컬 Windows VHDX를 보여주면 잘못된 정보가 되므로 unavailable 처리

따라서 이 기능의 가치는 “무조건 다 지우기”가 아니라, Docker 객체 reclaim과
host-side VHDX allocation을 분리해서 사용자가 잘못된 원인을 추적하지 않게 하는 데 있다.
