import { describe, expect, it } from 'vitest'
import { togglePelicanComparison } from '../compareSelection'

describe('pelican compare selection', () => {
  it('adds, removes, and caps the selection at two records', () => {
    const first = togglePelicanComparison([], 'a')
    const second = togglePelicanComparison(first, 'b')
    const capped = togglePelicanComparison(second, 'c')
    const removed = togglePelicanComparison(capped, 'a')

    expect(first).toEqual(['a'])
    expect(second).toEqual(['a', 'b'])
    expect(capped).toEqual(['a', 'b'])
    expect(removed).toEqual(['b'])
  })
})
