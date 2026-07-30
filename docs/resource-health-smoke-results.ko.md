# CPU/MEM health scenario smoke 결과

실행 일시: 2026-07-30 13:24 KST

명령:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scenarios\run-dockviz-resource-health.ps1 `
  -RunLabel codex-smoke `
  -DurationSeconds 30 `
  -SampleIntervalSeconds 2 `
  -OutputDirectory .\artifacts
```

생성된 project label:

```text
codex-smoke-dockviz-health-20260730042422
```

결과 파일:

- `artifacts/dockviz-health-20260730042422-samples.csv`
- `artifacts/dockviz-health-20260730042422-summary.json`

컨테이너별 Docker stats smoke 결과:

| Container | Samples | Max CPU | Max Memory |
|---|---:|---:|---:|
| `dockviz-health-20260730042422-cpu-hog` | 7 | 91.92% | 0.445 MB |
| `dockviz-health-20260730042422-mem-pressure` | 7 | 0% | 56.56 MB |
| `dockviz-health-20260730042422-mem-growth` | 7 | 3.75% | 80.64 MB |
| `dockviz-health-20260730042422-no-limit` | 7 | 0% | 0.434 MB |

예상 dockviz 확인 지점:

- Containers 패널 상단에 project resource summary가 표시된다.
- Problems 패널에 `High CPU` 또는 `CPU saturated`, `Memory pressure` 또는 `Memory over limit`, `Memory growth`,
  `No resource limits`가 표시된다.
- Problems 패널에서 `[Enter]`를 누르면 detail과 read-only recommendation이 표시된다.

정리 확인:

- `docker ps -a --filter "label=dockviz.scenario=dockviz-health-20260730042422"` 결과가 비어 있었다.
- 시나리오 컨테이너는 `finally` 블록에서 정확한 이름으로 `docker rm -f` 처리된다.
