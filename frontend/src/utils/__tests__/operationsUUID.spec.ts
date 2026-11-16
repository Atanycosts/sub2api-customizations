// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { operationsUUID } from '../operationsUUID'

afterEach(() => vi.unstubAllGlobals())

describe('operations submit identity', () => {
  it('uses the secure-context native UUID when available', () => {
    const randomUUID = vi.fn(() => 'b99cc106-c86a-4383-bd04-5e60bb70d562')
    vi.stubGlobal('crypto', { randomUUID })
    expect(operationsUUID()).toBe('b99cc106-c86a-4383-bd04-5e60bb70d562')
    expect(randomUUID).toHaveBeenCalledOnce()
  })

  it('generates a v4 UUID through getRandomValues on HTTP', () => {
    const getRandomValues = vi.fn((bytes: Uint8Array) => bytes.fill(255))
    vi.stubGlobal('crypto', { getRandomValues })
    expect(operationsUUID()).toBe('ffffffff-ffff-4fff-bfff-ffffffffffff')
    expect(getRandomValues).toHaveBeenCalledOnce()
  })
})
