# Fuel Ops Command Center — Engineering Workflow (v2)

> v2 replaces the first draft. It fixes several correctness problems and adds items the
> official brief asks for that v1 missed. Every simulator claim in §2 was **checked against the
> running simulator** (image `asifmahmoud414/bup-fuel-supply-simulator:1.0.0`) on 2026-09-29.
> Source priority: official brief & integration guide > measured behaviour > this document.

Goal: **top 3**. Time: **6 hours**. Team: planned for 3 developers (A/B/C); with fewer people,
the tracks below are merged in order A → B → C.

---

## 0. What changed from v1

| # | v1 said | v2 says | Why |
|---|---|---|---|
| 1 | Operator approves every recommendation | **Autopilot with guardrails** + human review for consequential decisions | At 8 ticks/s a simulated day passes in 12 s; a "stockout in 5 ticks" warning expires before anyone clicks |
| 2 | Run the engine when the dashboard refreshes | **One server-side control loop** (SSE-triggered + polling fallback) that caches state and recommendations | Otherwise nothing is monitored with no browser open, two tabs double-execute, and k6 hammers the simulator |
| 3 | Idempotency key `runId-tick-station-fuel-uuid` | Key generated **once per planned shipment** and reused on every retry | A fresh uuid per retry duplicates a shipment whose first POST timed out but succeeded |
| 4 | Lead time = `transit_ticks` (from docs example) | Created at tick T (paused) → arrives in tick **T + transit**; plan with `transit + 1` of demand as safety | Measured (§2) |
| 5 | Each recommendation checked alone | **Batch planner with shared budgets** (depot inventory, depot dispatch/tick incl. PENDING, station headroom) | `DISPATCH_CAPACITY_EXCEEDED` counts all pending + in-flight from the depot this tick (measured) |
| 6 | Weighted-moving-average forecast | **Model-based forecast** from the documented demand model, calibrated online | Demand = profile × hour factor × region × multiplier × ~±10% noise (measured, exact); WMA lags 2–3× jumps at shift changes |
| 7 | React to events when they happen | **Look ahead**: `/v1/events` shows `SCHEDULED` events with `start_tick` | Avoid routes before they are disrupted, pre-stock before a demand spike |
| 8 | `/v1/state` endpoint | Does not exist | — |
| 9 | Error parsing unspecified | `detail.code` (domain 4xx), `error.code` (fault 503), `detail.code` (stream fault), `detail: [...]` (422) | Guide §9 |
| 10 | Strategy comparison anytime | Runs `reset` + stepping → **wipes live state**; run before the demo, store results | Single-tenant simulator |
| 11 | Loop OBSERVE→…→EXPLAIN→ACT | Brief's loop: Observe → Detect → Predict → Decide → **Simulate** → Act → Monitor → Recover | Brief §2 — add a what-if preview |
| 12 | — | Alternatives + confidence/stockout probability on every recommendation | Brief §9 |
| 13 | — | CI/CD (GitHub Actions), system CPU/memory, p95/error-rate, alerts feed, scenario control panel | Brief §12, §14, §15, §18 |
| 14 | `prisma` "latest" | **Pin Prisma 7.10.0** | npm `latest` tag currently points to `8.0.0-rc.17` |
| 15 | — | Depot overflow: supply above depot capacity is lost | Measured: with no shipments depots sit at capacity by day 2 |

---

## 1. What we are building

An operator-facing **Fuel Supply Intelligence & Resilience Platform** on top of the organizer's
simulator. The simulator is the world; our system is the brain.

Winning story (one sentence a judge remembers):

> "The system forecast Tongi diesel would run out at 06:00 when the industrial shift starts,
> shipped 6,500 L from Gazipur two ticks earlier, re-routed when the Gazipur→Mirpur route was
> disrupted, kept working through API faults — and over a deterministic 2-day run it raised
> service level from 46% (no action) to X% (measured)."

Judging weights (brief p.10): Product & UX 20 · Intelligence 20 · Architecture 15 · DevOps 15 ·
Resilience 10 · Observability 10 · Demo 10. **A balanced, working system beats a clever model.**

---

## 2. Verified simulator facts (measured 2026-09-29)

**World:** 2 regions, 2 depots, 4 stations, 6 routes, DIESEL/PETROL/OCTANE, 15-min ticks,
22 supply arrivals (all to depots; 4 initial at ticks 12–20, then every 64 ticks).

| Fact | Measured |
|---|---|
| `sim_time` format | `"2026-01-01T00:15:00"` — **no timezone suffix** (guide shows `+00:00`) → parse as UTC |
| Idempotent replay (same key, same body) | **201** (guide §9 says 200 — accept both) |
| Same key, different body | 409 `{"detail":{"code":"IDEMPOTENCY_KEY_MISMATCH",...}}` |
| Dispatch capacity | Gazipur 3,000 + 6,500 PENDING accepted; +2,600 → 409 `DISPATCH_CAPACITY_EXCEEDED` (12,100 > 12,000). PENDING counts. |
| Validation order | Dispatch capacity is checked **before** destination capacity |
| Lifecycle (paused, created at tick 0, transit 2) | step→tick1: IN_TRANSIT (departure 0, expected 2) · step→tick3: ARRIVED (actual 2) |
| Arrival | Fuel lands during processing of tick `created + transit`; demand of that tick is also consumed |
| `/v1/demand-history` | Returned newest-first; `limit` = most recent rows; 12 rows/tick |
| Speed | 192 `/admin/step` calls ≈ 4 s → a 2-day experiment takes seconds |

**Demand model (exact, verified on 2,304 observations):**

```
demand_per_tick = daily_profile[fuel] / (1440 / tick_minutes)
                × hour_factor(profile, hour)
                × region.demand_factor         (dhaka 1.00, chattogram 1.08)
                × station.demand_multiplier    (changed live by demand_spike)
                × noise                        (≈ ±8–12 %, per profile)
```

Hour factors (inclusive hours):

| profile | busy hours → factor | otherwise |
|---|---|---|
| urban_high (Mirpur) | 07–09, 16–20 → 1.45 | 0.70 |
| industrial (Tongi) | 06–17 → 1.55 | 0.45 |
| highway (Karnaphuli) | 06–09, 16–20 → 1.35 | 0.75 |
| regional (Cox's Bazar) | 07–20 → 1.25 | 0.65 |

**No-action baseline (2 days, 192 ticks, default scenario):** service level **0.461**,
unmet 100,452 L. First stockouts: Tongi DIESEL t65, Karnaphuli DIESEL t74, Karnaphuli OCTANE
t76, Karnaphuli PETROL t79, Cox's PETROL t82, Mirpur PETROL t83 … Depots reach capacity
(Gazipur 90,000 D) → **later supply is wasted** unless we ship.

---

## 3. The engineering loop (brief §2)

```
OBSERVE   control loop pulls REST snapshot (SSE tick = trigger, REST = truth)
DETECT    anomalies, active/scheduled events, stale data, faults, low coverage
PREDICT   hour-aware demand forecast → inventory trajectory → stockout tick + probability
DECIDE    batch planner: candidates → hard constraints → score → shared budgets
SIMULATE  what-if: projected inventory & unmet demand with vs without the plan
ACT       autopilot (safe) or operator approval (consequential) → POST /v1/allocations
MONITOR   allocation lifecycle, forecast error, service level, health, latency
RECOVER   retry/backoff, last-known-good, fallback engine, cancel doomed PENDING, re-plan
```

---

## 4. Architecture

```
┌───────────────────────────── docker compose ─────────────────────────────┐
│                                                                          │
│  simulator-api :8000  (organizer image, unchanged)                       │
│        ▲ REST (+ /admin for scenario controls)      │ SSE /v1/stream     │
│        │                                            ▼                    │
│  ┌──────────────────────── app :3000 (Next.js, Node runtime) ─────────┐  │
│  │ SimulatorClient  timeout · retry(503/timeout) · stale header ·     │  │
│  │                  error parsing · latency metrics                   │  │
│  │        ▼                                                           │  │
│  │ Control loop (singleton, started in instrumentation.ts)            │  │
│  │   trigger: SSE simulation.tick (coalesced) | poll every 2 s        │  │
│  │   1 snapshot = parallel GETs → WorldState (+ last-known-good)      │  │
│  │   2 intelligence: forecast → projection → risk → planner → what-if │  │
│  │     (fallback: baseline threshold engine if primary throws)        │  │
│  │   3 autopilot policy → execute / queue for operator                │  │
│  │   4 publish to in-memory store → browser SSE /api/stream           │  │
│  │   5 persist async (non-blocking) → Postgres via Prisma 7           │  │
│  │        ▼                                                           │  │
│  │ /api/*  state · recommendations · decisions · allocations ·        │  │
│  │         health · metrics(Prometheus) · stream · scenario · exp.    │  │
│  │ UI      command-center dashboard (React, Tailwind, Recharts)       │  │
│  └───────────────────────────────┬────────────────────────────────────┘  │
│                                  ▼                                       │
│  postgres :5432   decisions, recommendations, incidents, experiments     │
└──────────────────────────────────────────────────────────────────────────┘
```

Key decisions:
- **One Next.js app** (UI + API + loop). No microservices.
- **The loop is the only thing that talks to the simulator on a schedule.** API routes serve the
  cached snapshot → load tests measure *our* system, not the simulator, and the simulator is
  never multiplied by client count.
- **At 8 ticks/s**, the loop coalesces: if a cycle is running, the next trigger only sets a
  "dirty" flag. Each cycle must finish in < 125 ms locally; it is measured and exported.
- **DB is optional at runtime.** If Postgres is down, the loop keeps running; health shows
  Database DEGRADED; writes are dropped with a counted warning.
- **REST is truth, SSE is a hint.** After SSE reconnect → immediate full refresh.

---

## 5. Tech stack (pinned)

| Concern | Choice | Version |
|---|---|---|
| App | Next.js App Router, TypeScript, `src/` dir | next 16.3.x, react 19 |
| Styling | Tailwind CSS v4 (+ shadcn/ui if time) | 4.x |
| Charts | Recharts | 3.x |
| Validation | Zod (env, simulator responses, API input) | 4.x |
| ORM | **Prisma 7** `prisma-client` generator + `@prisma/adapter-pg` | **7.10.0 exactly** |
| DB | PostgreSQL 17 (compose) | — |
| Logs | Tiny structured JSON logger (stdout) | own code |
| Metrics | In-process registry → `/api/metrics` (Prometheus text) + in-app panel | own code |
| Tests | Vitest | — |
| Load | k6 via `grafana/k6` Docker image (no install) | — |
| CI | GitHub Actions | — |

### Prisma 7 layout (team decision)

```
prisma/schema.prisma          generator client { provider = "prisma-client"
                                                 output   = "../src/generated/prisma" }
prisma.config.ts              schema path, migrations path, datasource url (env DATABASE_URL)
src/generated/prisma/         generated client — gitignored, created by `prisma generate`
src/lib/prisma.ts             singleton PrismaClient({ adapter: new PrismaPg(...) }),
                              lazily created, cached on globalThis (dev HMR safe)
```

Notes: in Prisma 7 the datasource URL lives in `prisma.config.ts` (not the schema), `.env` is
not auto-loaded (config imports `dotenv/config`), and a driver adapter is required.
`prisma generate` runs in `postinstall` and in the Docker build. Import path:
`import { PrismaClient } from '@/generated/prisma/client'`.

---

## 6. Folder structure

```
.
├── .github/workflows/ci.yml
├── docker-compose.yml          simulator (unchanged) + postgres + app
├── Dockerfile                  multi-stage, Next standalone output
├── prisma.config.ts
├── prisma/schema.prisma
├── load/                       k6 scripts + results
├── docs/                       architecture, resilience evidence, load-test report
├── src/
│   ├── instrumentation.ts      starts the control loop (nodejs runtime only)
│   ├── app/
│   │   ├── page.tsx            command center
│   │   └── api/{state,recommendations,decisions,allocations,health,metrics,
│   │           stream,scenario,experiments}/route.ts
│   ├── components/dashboard/…
│   ├── generated/prisma/       (gitignored)
│   └── lib/
│       ├── config.ts           zod-validated env
│       ├── prisma.ts
│       ├── simulator/{client,types,errors}.ts
│       ├── world/snapshot.ts   WorldState + last-known-good
│       ├── intelligence/{demand-model,forecast,projection,risk,planner,baseline,
│       │                 whatif,explain}.ts
│       ├── decisions/{autopilot,execute}.ts
│       ├── engine/loop.ts      control loop + in-memory store
│       └── observability/{logger,metrics,health}.ts
└── tests/…
```

---

## 7. Data model (Prisma) — only what the simulator does not store

| Model | Purpose |
|---|---|
| `Recommendation` | tick, station, fuel, risk before/after, stockout prob before/after, forecast, plan (depot/route/qty), explanation JSON, alternatives JSON, engine (`primary`/`baseline`), status |
| `Decision` | recommendation id, action (`APPROVED`/`REJECTED`/`AUTO_EXECUTED`/`MODIFIED`), actor, reason |
| `AllocationExecution` | idempotency key, simulator allocation id, request, response code, final status, arrival tick |
| `Incident` | kind (`SIM_UNAVAILABLE`, `STALE_DATA`, `SSE_DOWN`, `ENGINE_FALLBACK`, `DB_DOWN`, `CRISIS_EVENT`…), severity, opened/closed at, details |
| `MetricSnapshot` | tick, service level, unmet, allocations, failures, forecast MAE (sampled, not every tick) |
| `Experiment` | strategy, ticks, scenario script, seed, results JSON |

Never copy depots/stations/routes/allocations as tables — the simulator owns them.

---

## 8. Application API

| Method | Path | Purpose | P |
|---|---|---|---|
| GET | `/api/health` | component health (app, simulator liveness, simulator data API, SSE, DB, engine), p95 latency, error rate, CPU/mem | P0 |
| GET | `/api/state` | cached WorldState + freshness (age, stale flag, source) | P0 |
| GET | `/api/recommendations` | current ranked plan with explanations, alternatives, what-if | P0 |
| POST | `/api/decisions` | approve / reject / modify a recommendation (revalidates vs latest state) | P0 |
| GET | `/api/allocations` | shipment ledger joined with our recommendations | P0 |
| GET | `/api/stream` | browser SSE: dashboard snapshot, throttled ≤ 2/s | P1 |
| GET | `/api/metrics` | Prometheus text: latency histograms, errors, retries, loop time, forecast MAE, decisions | P1 |
| POST | `/api/scenario` | step/pause/run/reset, inject event/fault (guarded by `ENABLE_SCENARIO_CONTROLS`) | P1 |
| PUT | `/api/autopilot` | mode `MANUAL` / `ASSISTED` / `AUTO` | P1 |
| POST | `/api/experiments` | run strategy comparison (locks loop, resets simulator) | P1 |
| GET | `/api/history` | decisions + incidents timeline | P1 |

All inputs validated with Zod; errors return `{ error: { code, message } }`.

---

## 9. Intelligence design

### 9.1 Forecast (PREDICT)
- `model(t) = daily/96 × hourFactor(hour(t)) × regionFactor × demand_multiplier(t)`.
- Future `demand_multiplier(t)`: current value, × multiplier for ticks inside a **SCHEDULED**
  demand_spike window, ÷ multiplier after an **ACTIVE** spike's `end_tick`.
- Online calibration per station×fuel: `k = EWMA(actual / model)` over recent ticks (clamped
  0.5–2.0) — absorbs anything the model misses.
- Track 1-tick-ahead forecast error → **MAE / MAPE** exported as an intelligence metric.

### 9.2 Projection (PREDICT → SIMULATE)
For each station×fuel, tick-by-tick over horizon `H = 24` ticks (6 h):
`inv[t+1] = inv[t] − forecast[t] + arrivals[t]` where arrivals = our IN_TRANSIT/PENDING
allocations at `expected_arrival_tick` (PENDING: `created + transit`).
Outputs: first tick below safety stock, first stockout tick, projected unmet liters.

### 9.3 Risk & uncertainty (DETECT)
- Stockout probability before a shipment could land: demand over lead time
  `D ~ N(μ, σ²)`, `σ² = Σ (noise·f_t)² + (0.05·μ)²` (model error) →
  `P = 1 − Φ((inv + inbound − μ)/σ)`.
- Risk score 0–100 = 45 % stockout urgency (ticks to breach vs lead time) + 25 % stockout
  probability + 15 % inventory % + 10 % crisis exposure (active/scheduled events touching the
  station/its routes/depot) + 5 % route fragility (only one AVAILABLE route).
- Levels: 0–29 LOW, 30–59 MEDIUM, 60–79 HIGH, 80–100 CRITICAL. Weights in one config object.

### 9.4 Batch planner (DECIDE)
Order-up-to policy with shared budgets, run every cycle:
1. Needs: station×fuel where projected inventory at `now + lead + review(4 ticks)` < safety
   stock (`3 ticks of busy-hour demand`). Sort by urgency.
2. Candidates per need: every route to the station. **Hard filters** (never scored):
   station OPEN, route AVAILABLE and not disrupted by a SCHEDULED/ACTIVE event at departure,
   depot OPEN/CONSTRAINED, remaining depot inventory, remaining depot dispatch this tick,
   `qty ≤ capacity − inventory − inbound` (creation check + no overflow at arrival).
3. Quantity: fill to order-up-to level (≈ 90 % capacity), split above `max_shipment` into
   several allocations, respect remaining dispatch budget.
4. Score valid candidates: shortage reduced (35) + arrives before stockout (25) + ETA (15) +
   depot health after shipment incl. **overflow avoidance** when supply arrives soon (15) −
   penalties: CONSTRAINED depot, cross-region, would starve a higher-risk need at the same
   depot (10).
5. Deduct budgets, continue with the next need. Depot reserve: never drop a depot below the
   projected needs of its higher-risk stations before the next supply arrival.

### 9.5 What-if (SIMULATE)
Re-run the projection with the plan applied: risk after, stockout probability after,
unmet liters avoided. Shown as "Risk 87 → 22 · P(stockout) 72 % → 4 %".

### 9.6 Explanation & alternatives
Built from the actual variables, e.g.:
```
Tongi / DIESEL — CRITICAL 91
Inventory 3,120 L (17 %) · forecast next 8 ticks 4,650 L (industrial shift starts 06:00, ×1.55)
Stockout in 5 ticks (P = 83 %) · Gazipur→Tongi ETA 2 ticks
Ship 6,500 L Gazipur→Tongi  →  risk 91 → 18, P(stockout) 83 % → 3 %
Alternatives: none via Patiya (no route) · smaller 3,000 L leaves P = 41 %
Confidence: HIGH (forecast MAPE 6 %, fresh data)
```

### 9.7 Autopilot policy (ACT)
| Mode | Behaviour |
|---|---|
| MANUAL | everything waits for the operator |
| **ASSISTED (default)** | auto-execute routine shipments; **queue for human review** when: confidence LOW, cross-region route, depot would fall below reserve, quantity > 6,000 L, or crisis active |
| AUTO | execute all valid plans |

Auto-execution is **suspended** when data is stale, simulator data API is degraded, or the
fallback engine is active (brief §11: "prediction confidence too low → human review").

### 9.8 Fallback engine
If the primary engine throws or times out: baseline threshold rule (inventory < 35 % → refill
from same-region depot on the shortest AVAILABLE route, capped by all hard constraints).
Incident `ENGINE_FALLBACK` is opened and shown.

### 9.9 Optional later
RL / bandits only if everything else is done; must be compared against the planner.

---

## 10. Crisis playbook

| Event | Detection | Response shown in UI |
|---|---|---|
| demand_spike | SCHEDULED/ACTIVE event; `demand_multiplier` ≠ 1 | forecast ×m inside window, risk jumps, pre-stock before start |
| route_disruption | event + route status | exclude route for ticks in window, **cancel PENDING** on it, switch to alternate (e.g. Patiya→Mirpur 4 ticks) |
| station_outage | event + station OUTAGE | stop shipping there, cancel PENDING to it, redistribute depot stock |
| depot_constraint | depot CONSTRAINED (still shippable) | penalty, keep larger reserve, prefer other depot |
| shipment_delay | supply-arrivals `planned_tick` moved, DELAYED | depot projection updates, ration scarce fuel by risk |
| supply_shortfall | supply quantity reduced | depot projection updates, reprioritise |
| combined | ≥ 2 active | all of the above + banner "COMBINED CRISIS" |

Every event opens an Incident with start/end and our response actions.

---

## 11. Resilience playbook (brief §11)

| Failure | Detection | Behaviour |
|---|---|---|
| latency fault | request duration | timeout `2500 ms`, retry ×2 (250 → 500 ms + jitter), latency panel turns amber |
| unavailable / error_rate (503 `error.code=FAULT_INJECTED`) | status + body | retry; if still failing → serve **last-known-good** with age, `SIMULATOR DATA API DEGRADED`; `/v1/health` still OK ⇒ "process alive, API faulted" |
| stale_data (`X-Simulator-Stale: true`) | header | keep last-known-good, **STALE** banner, pause auto-execution, keep polling |
| stream_disconnect (503 `detail.code`) / silent drop | SSE error / no tick for > 20 s while RUNNING | SSE DEGRADED, reconnect with backoff, **polling continues**, full refresh on reconnect |
| invalid simulator payload | Zod parse fails | reject snapshot, alert, keep last-known-good |
| POST allocation timeout | timeout | retry with the **same idempotency key**; 409 → no retry, re-plan |
| Postgres down | Prisma error | DB DEGRADED, continue in memory |
| engine exception | try/catch + time budget | fallback engine + incident |

Circuit breaker (simple): after 5 consecutive failures, skip data calls for 3 s, probe with
`/v1/health`.

---

## 12. Observability (brief §14–15)

- **Application:** request rate, latency p50/p95/p99, error rate per route (our API) and per
  simulator endpoint; retries; circuit state.
- **System:** process CPU %, RSS/heap memory, event-loop lag, uptime.
- **Intelligence:** forecast MAE/MAPE, recommendations/tick, HIGH+CRITICAL count, auto vs
  human decisions, approvals/rejections, fallback activations, loop duration.
- **Operations:** service level, unmet liters, allocation liters, failures (from simulator).
- **Logs:** JSON lines to stdout: `loop.cycle`, `recommendation.generated`,
  `allocation.executed|failed|cancelled`, `simulator.stale`, `simulator.degraded`,
  `sse.disconnected|reconnected`, `engine.fallback`, `crisis.detected|resolved`, `db.down|up`.
- **Alerts feed:** incidents + threshold alerts in the UI.
- P2: Prometheus + Grafana as a compose profile scraping `/api/metrics`.

---

## 13. DevOps (brief §12)

- `docker compose up -d --build` starts simulator + postgres + app; app waits for DB health,
  runs `prisma migrate deploy`, then starts; healthchecks on postgres and app.
- Dockerfile: multi-stage (deps → build with `prisma generate` + `next build` standalone →
  slim runtime, non-root).
- CI (GitHub Actions): install → lint → typecheck → unit tests → build → docker build →
  compose up → wait for `/api/health` → k6 smoke → compose down.
- Config only via env (`.env.example` documented); no secrets in code.

---

## 14. Load testing (brief §17)

k6 via Docker (`grafana/k6`) against the compose network. Scenarios:
1. Dashboard read path: `/api/state`, `/api/recommendations`, `/api/health` — ramp 10 → 100 VUs.
2. Decision path: `POST /api/decisions` dry-run (revalidate + what-if, no execution).
3. Same as 1 **during a latency fault** → shows the cache isolates users from simulator latency.

Report avg/p50/p95/p99, throughput, error rate, VUs, CPU/memory (`docker stats`) in
`docs/load-test.md` with the raw k6 summary JSON.

---

## 15. Strategy evaluation (evidence of intelligence)

`POST /api/experiments` → for each strategy: `reset` → `pause` → for N ticks {plan → execute →
`step`} → read `/v1/metrics`. Same seed, same scripted events, same N.

Strategies: A no action · B naive threshold (< 35 % refill on nearest route) · C our planner.
Scenarios: normal 192 ticks; crisis script (demand spike Dhaka ×1.8 t40–t64 + route disruption
Gazipur→Mirpur t50–t70 + shipment delay).

| Strategy | Service level | Unmet L | Failures | Shipped L |
|---|---:|---:|---:|---:|
| A No action (measured) | 0.461 | 100,452 | 0 | 0 |
| B Naive | measure | measure | measure | measure |
| C Planner | measure | measure | measure | measure |

Never fill numbers by hand. Run it before the demo (it resets the simulator).

---

## 16. Timeline (6 h, 3 devs)

| Time | A — Simulator/Backend/DevOps | B — Intelligence | C — UI/Demo | Checkpoint |
|---|---|---|---|---|
| 0:00–0:45 | **Step 1**: scaffold, client, snapshot, health/state, Prisma, compose, Dockerfile | demand model + unit tests | UI shell, KPI cards on `/api/state` | `/api/state` returns live world |
| 0:45–1:45 | control loop, execute + idempotency, SSE in, `/api/stream` | projection, risk, planner, what-if, explanations | station/fuel risk grid, recommendation card | one recommendation → approve → ARRIVED |
| 1:45–2:45 | autopilot modes, cancellations, scenario API | event look-ahead, baseline fallback | network/routes panel, charts, ledger, scenario panel | demand spike changes plan |
| 2:45–3:30 | retry/circuit/stale/LKG, incidents, DB degrade | forecast MAE, confidence | health panel, banners, alerts feed | faults don't break app |
| 3:30–4:15 | `/api/metrics`, CI workflow | experiments runner | timeline/history view | CI green, experiment table |
| 4:15–5:00 | k6 runs + report | run experiments, tune weights | polish, empty/error states | evidence captured |
| 5:00–5:30 | README, architecture diagram, docs | docs of intelligence | screenshots | `docker compose up` from clean clone |
| 5:30–6:00 | **freeze**, rehearse demo ×2 | rehearse | rehearse | demo succeeds twice |

Merge to `main` every 30–60 min. Shared contracts first: `WorldState`, `Recommendation`,
`HealthStatus` in `src/lib/simulator/types.ts` and `src/lib/intelligence/types.ts`.

---

## 17. Demo script (maps to brief §22, ~5 min)

Setup: `SIMULATION_SPEED=2`, reset, autopilot ASSISTED, experiments already stored.

1–2. Normal operations: dashboard, all health green, service level 100 %.
3–5. Run → morning shift: Tongi DIESEL risk climbs; forecast shows the 06:00 ×1.55 jump.
6–7. Recommendation card: why, constraints, alternatives, what-if 91 → 18.
8. Approve (or show autopilot executed it) → PENDING → IN_TRANSIT → ARRIVED.
9–10. Inject route disruption Gazipur→Mirpur + Dhaka demand spike from the scenario panel →
     CRISIS banner, PENDING cancelled, re-routed via Patiya, risk re-ranked.
11–13. Inject `unavailable` then `stale_data` → DEGRADED/STALE banners, last-known-good with
     age, autopilot suspended, health shows simulator alive but API faulted; clear → recovery.
14. Operations continue; show experiment table (A vs B vs C), k6 report, CI run, metrics.

---

## 18. Priority board

**P0:** simulator client + resilience · snapshot/LKG · forecast · projection · risk · planner ·
explanation + what-if · approve/execute · control loop · lifecycle · health panel ·
stale/503 handling · crisis detection + one adaptation · compose + README · demo.

**P1:** autopilot modes · cancellations · event look-ahead · SSE in/out · decision history (DB) ·
experiments · k6 · CI · `/api/metrics` · alerts feed · scenario panel.

**P2:** Prometheus/Grafana profile · shadcn polish · LLM incident summaries · RL/bandit.

**Do not build:** auth/roles, chatbot, vector DB, Kubernetes, microservices, deep learning.

---

## 19. Risks

| Risk | Mitigation |
|---|---|
| Sim too fast for demo | `SIMULATION_SPEED=2` for demo; scenario panel step/pause |
| Invalid allocations | hard filters mirror simulator validation order; 409 → re-plan, never retry |
| Double execution | single loop + deterministic idempotency keys |
| Prisma 7 friction | pinned 7.10.0; DB optional at runtime |
| Experiment wipes demo state | run before demo; loop locked during run |
| Time | P0 freeze at 4:15; no refactors in last 90 min |

---

## 20. Environment

```env
SIMULATOR_BASE_URL=http://localhost:8000      # compose: http://simulator-api:8000
DATABASE_URL=postgresql://fuel:fuel@localhost:5432/fuel_ops
SIMULATOR_TIMEOUT_MS=2500
SIMULATOR_MAX_RETRIES=2
LOOP_POLL_MS=2000
DEMAND_HISTORY_TICKS=32
AUTOPILOT_MODE=ASSISTED
ENABLE_SCENARIO_CONTROLS=true
LOG_LEVEL=info
```
