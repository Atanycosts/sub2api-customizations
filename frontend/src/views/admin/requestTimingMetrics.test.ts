import { describe, expect, it } from 'vitest'
import { requestTimingMetrics } from './requestTimingMetrics'
import type { RequestTiming } from '@/types/requestTimings'
const sample = { duration_ms: 100, admission_complete: true, admission: [{kind:'user',start_ms:0,end_ms:20},{kind:'account',start_ms:10,end_ms:30}], attempts: [{start_ms:30,end_ms:50},{start_ms:55,end_ms:100}] } as RequestTiming

describe('request timing measured aggregates', () => {
  it('unions overlapping waits and keeps preceding attempts separate', () => {
    expect(requestTimingMetrics(sample)).toEqual({ measuredWait:30,wait:30,withoutWait:70,finalAttempt:45,previousAttempts:20 })
  })
  it('requires complete wait coverage before subtracting waits', () => {
    const result = requestTimingMetrics({...sample,admission_complete:false,admission:[]})
    expect(result.wait).toBeNull(); expect(result.withoutWait).toBeNull()
  })
  it('preserves measured zero wait and missing attempt endpoints', () => {
    expect(requestTimingMetrics({...sample,admission:[]}).wait).toBe(0)
    expect(requestTimingMetrics({...sample,attempts:[{...sample.attempts[0]!,end_ms:null},sample.attempts[1]!]}).previousAttempts).toBeNull()
  })
})
