import { describe, expect, it } from 'vitest'
import {
  canRunPelicanTests,
  clampPage,
  pelicanPlanInput,
  pelicanSubmitID,
  pelicanViewportRows,
} from '../pelicanAdmin'
import type { PelicanPlan } from '@/types/pelican'

const plan: PelicanPlan = {
  id: 9,
  group_id: 10,
  group_name: 'OpenAI API',
  model: 'custom-model-preserved',
  effort: 'high',
  protocol: 'responses',
  enabled: true,
  published: false,
  interval_minutes: 240,
  max_output_tokens: 8192,
  budget_usd: 1.25,
  last_run_at: null,
  next_run_at: '2026-11-18T12:00:00Z',
  running_until: null,
}

describe('pelican admin behavior', () => {
  it('gates manual execution on the saved config values', () => {
    expect(canRunPelicanTests({ enabled: true, daily_budget_usd: 0 })).toBe(false)
    expect(canRunPelicanTests({ enabled: false, daily_budget_usd: 10 })).toBe(false)
    expect(canRunPelicanTests({ enabled: true, daily_budget_usd: 0.01 })).toBe(true)
  })

  it('sends every editable plan field while preserving custom models', () => {
    expect(pelicanPlanInput(plan, { published: true })).toEqual({
      group_id: 10,
      model: 'custom-model-preserved',
      effort: 'high',
      protocol: 'responses',
      enabled: true,
      published: true,
      interval_minutes: 240,
      max_output_tokens: 8192,
      budget_usd: 1.25,
    })
  })

  it('retains a manual submit ID across a deliberate retry', () => {
    const first = pelicanSubmitID(undefined, () => 'stable-id')
    expect(pelicanSubmitID(first, () => 'replacement-id')).toBe('stable-id')
  })

  it('uses the measured page height for responsive capacities and clamps page indexes', () => {
    expect(pelicanViewportRows(1920, 984)).toEqual({ plans: 8, results: 7 })
    expect(pelicanViewportRows(1440, 804)).toEqual({ plans: 6, results: 5 })
    expect(pelicanViewportRows(1280, 704)).toEqual({ plans: 5, results: 4 })
    expect(pelicanViewportRows(1366, 672)).toEqual({ plans: 5, results: 3 })
    expect(pelicanViewportRows(390, 552)).toEqual({ plans: 3, results: 2 })
    expect(clampPage(9, 11, 5)).toBe(3)
  })
})
