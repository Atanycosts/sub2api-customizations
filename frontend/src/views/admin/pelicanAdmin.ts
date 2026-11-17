import type { PelicanConfig, PelicanPlan, PelicanPlanInput } from '@/types/pelican'

export interface PelicanViewportRows {
  plans: number
  results: number
}

export function canRunPelicanTests(config: Pick<PelicanConfig, 'enabled' | 'daily_budget_usd'>): boolean {
  return config.enabled && config.daily_budget_usd > 0
}

export function pelicanPlanInput(
  plan: PelicanPlan,
  overrides: Partial<Pick<PelicanPlanInput, 'enabled' | 'published'>> = {},
): PelicanPlanInput {
  return {
    group_id: plan.group_id,
    model: plan.model,
    effort: plan.effort,
    protocol: plan.protocol,
    enabled: overrides.enabled ?? plan.enabled,
    published: overrides.published ?? plan.published,
    interval_minutes: plan.interval_minutes,
    max_output_tokens: plan.max_output_tokens,
    budget_usd: plan.budget_usd,
  }
}

export function pelicanSubmitID(existing: string | undefined, create: () => string): string {
  return existing || create()
}

export function pelicanViewportRows(width: number, pageHeight: number): PelicanViewportRows {
  if (width <= 620) return { plans: 3, results: 2 }
  if (pageHeight <= 690) return { plans: 5, results: 3 }
  if (pageHeight <= 720) return { plans: 5, results: 4 }
  if (pageHeight <= 840) return { plans: 6, results: 5 }
  return { plans: 8, results: 7 }
}

export function clampPage(page: number, total: number, pageSize: number): number {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return Math.min(Math.max(1, page), pages)
}
