# Task 1 Report: Update GPT-5.6 Luna Mapping

## Status
PASS — all acceptance criteria met.

## Commit
`81e5f24` — `fix: update gpt-5.6-luna fallback to gpt-5.4`

## Files changed (2)
- `internal/modelversion/fallback.go` — `gpt56CodenameFallback["gpt-5.6-luna"]` → `gpt-5.4` (was `gpt-5.4-mini`)
- `internal/modelversion/fallback_test.go` — `TestNextFallbackModel` luna expects `gpt-5.4`

## TDD evidence

### RED: test updated before production change
```
--- FAIL: TestNextFallbackModel (0.00s)
    --- PASS: .../gpt-5.6-terra (0.00s)
    --- PASS: .../gpt-5.6-sol (0.00s)
    --- FAIL: .../gpt-5.6-luna (0.00s)
        fallback_test.go:68: Next("gpt-5.6-luna") = "gpt-5.4-mini", want "gpt-5.4"
```

### GREEN: production change aligns with test
PASS `TestNextFallbackModel` (29/29, all sub-tests including `gpt-5.6-luna`)

## Focused test results
```
go test ./internalmodelversion -run 'Test(NextFallbackModel|NextGPT56Codename|ChainGPT56)' -count=1 -v
```
- `TestNextFallbackModel` — 29/29 PASS
- `TestNextGPT56CodenameFallback` — 2/2 PASS
- `TestChainGPT56Codename` — 2/2 PASS

## Concerns
None. Mapping is now exact literal per product policy. No other files touched.

## Report path
`.superpowers/sdd/task-1-report.md`